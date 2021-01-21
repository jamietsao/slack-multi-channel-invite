package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

const (
	conversationsInviteURL = "https://slack.com/api/conversations.invite"
	conversationsListURL   = "https://slack.com/api/conversations.list"
	usersLookupByEmailURL  = "https://slack.com/api/users.lookupByEmail"
)

type (
	conversationsListResponse struct {
		Ok               bool             `json:"ok"`
		Channels         []channel        `json:"channels"`
		ResponseMetadata responseMetadata `json:"response_metadata"`
		Error            string           `json:error`
		Needed           string           `json:needed`
		Provided         string           `json:provided`
	}

	channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	responseMetadata struct {
		NextCursor string `json:"next_cursor"`
	}

	conversationsInviteRequest struct {
		ChannelID string `json:"channel"`
		UserIDs   string `json:"users"`
	}

	conversationsInviteResponse struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error"`
	}

	usersLookupByEmailResponse struct {
		Ok    bool   `json:"ok"`
		User  user   `json:"user"`
		Error string `json:"error"`
	}

	user struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
)

// This script invites the given user to the given list of channels on Slack.
// Due to the oddness of the Slack API, this is accomplished via these steps:
// 1) Look up the Slack user ID by email
// 2) Query all public (private if 'private' flag is set to true) channels in the workspace and create a name -> ID mapping
// 3) For each of the given channels, invite the user (user ID) to the channel (channel ID)
func main() {
	var apiToken string
	var userEmail string
	var channelsArg string
	var private bool
	var debug bool

	// parse flags
	flag.StringVar(&apiToken, "api_token", "", "Slack OAuth Access Token")
	flag.StringVar(&userEmail, "user_email", "", "Email of Slack user to invite")
	flag.StringVar(&channelsArg, "channels", "", "Comma separated list of channels to invite user to")
	flag.BoolVar(&private, "private", false, "Boolean flag to enable private channel invitations (requires OAuth scopes 'groups:read' and 'groups:write')")
	flag.BoolVar(&debug, "debug", false, "Enables debug logging when set to true")
	flag.Parse()

	if apiToken == "" || userEmail == "" || channelsArg == "" {
		flag.Usage()
		os.Exit(1)
	}

	// lookup user by email
	userID, err := getUserID(apiToken, userEmail)
	if err != nil {
		panic(err)
	}

	if debug {
		fmt.Printf("User ID for '%s': %s\n", userEmail, userID)
	}

	// get all channels
	channelNameToIDMap, err := getChannels(apiToken, private)
	if err != nil {
		panic(err)
	}

	if debug {
		fmt.Printf("# of channels retrieved: %d\n", len(channelNameToIDMap))
	}

	// invite user to each channel
	channels := strings.Split(channelsArg, ",")
	for _, channel := range channels {
		channelID := channelNameToIDMap[channel]
		if channelID == "" {
			fmt.Printf("Channel '%s' not found -- skipping\n", channel)
			continue
		}

		if debug {
			fmt.Printf("Inviting user %s (%s) to channel %s (%s)\n", userEmail, userID, channel, channelID)
		}

		err := inviteUserToChannel(apiToken, userID, channelID)
		if err != nil {
			fmt.Printf("Error while inviting %s (%s) to %s (%s): %s\n", userEmail, userID, channel, channelID, err)
			continue
		}

		fmt.Printf("User %s invited to '%s'\n", userEmail, channel)
	}

	fmt.Println("\nAll done! You're welcome =)")
}

func getUserID(apiToken, userEmail string) (string, error) {

	// lookup user by email
	resp, err := http.Get(usersLookupByEmailURL + fmt.Sprintf("?token=%s&email=%s", apiToken, userEmail))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := printErrorResponseBody(resp)
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("Non-200 status code (%d)", resp.StatusCode)
	}

	var data usersLookupByEmailResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return "", err
	}

	if !data.Ok {
		fmt.Printf("usersLookupByEmailResponse: %+v\n", data)
		return "", fmt.Errorf("Non-ok response while looking up user by email")
	}

	// return user ID
	return data.User.ID, nil
}

// TODO: add proper paging to ensure all channels are retrieved
func getChannels(apiToken string, private bool) (map[string]string, error) {

	channelType := "public_channel"
	if private {
		channelType = "private_channel"
	}

	// query list of channels
	resp, err := http.Get(conversationsListURL + fmt.Sprintf("?token=%s&exclude_archived=true&limit=1000&types=%s", apiToken, channelType))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := printErrorResponseBody(resp)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("Non-200 status code (%d)", resp.StatusCode)
	}

	var data conversationsListResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return nil, err
	}

	if !data.Ok {
		fmt.Printf("conversationsListResponse: %+v", data)
		return nil, fmt.Errorf("Non-ok response while querying list of channels")
	}

	// create map of channel names to IDs
	nameToID := make(map[string]string)
	for _, channel := range data.Channels {
		nameToID[channel.Name] = channel.ID
	}

	return nameToID, nil
}

func inviteUserToChannel(apiToken, userID, channelID string) error {
	httpClient := &http.Client{}

	reqBody, err := json.Marshal(conversationsInviteRequest{
		ChannelID: channelID,
		UserIDs:   userID,
	})
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodPost, conversationsInviteURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := printErrorResponseBody(resp)
		if err != nil {
			return err
		}
		return fmt.Errorf("Non-200 status code: (%d)", resp.StatusCode)
	}

	var data conversationsInviteResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return err
	}

	if !data.Ok {
		fmt.Printf("conversationsInviteResponse: %+v\n", data)
		return fmt.Errorf("Non-ok response while inviting user to channel")
	}

	return nil
}

func printErrorResponseBody(resp *http.Response) error {
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Println(string(bodyBytes))

	return nil
}
