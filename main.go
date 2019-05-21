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
// 2) Query all public channels in the workspace and create a name -> ID mapping
// 3) For each of the given channels, invite the user (user ID) to the channel (channel ID)
func main() {
	var apiToken string
	var userEmail string
	var channelsArg string

	// parse flags
	flag.StringVar(&apiToken, "api_token", "", "Slack OAuth Access Token")
	flag.StringVar(&userEmail, "user_email", "", "Email of Slack user to invite")
	flag.StringVar(&channelsArg, "channels", "", "Comma separated list of channels to invite user to")
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

	// get all public channels
	channelNameToIDMap, err := getPublicChannels(apiToken)
	if err != nil {
		panic(err)
	}

	// invite user to each channel
	channels := strings.Split(channelsArg, ",")
	for _, channel := range channels {
		channelID := channelNameToIDMap[channel]

		err := inviteUserToChannel(apiToken, userID, userEmail, channelID, channel)
		if err != nil {
			fmt.Printf("Error while inviting %s to %s: %s\n", userEmail, channel, err)
		}
	}

	fmt.Println("All done! You're welcome =)")
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

func getPublicChannels(apiToken string) (map[string]string, error) {
	// query list of public channels
	resp, err := http.Get(conversationsListURL + fmt.Sprintf("?token=%s&exclude_archived=true&limit=1000", apiToken))
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
		return nil, fmt.Errorf("Non-ok response while querying list of public channels")
	}

	// create map of channel names to IDs
	nameToID := make(map[string]string)
	for _, channel := range data.Channels {
		nameToID[channel.Name] = channel.ID
	}

	return nameToID, nil
}

func inviteUserToChannel(apiToken, userID, userEmail, channelID, channelName string) error {
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

	fmt.Printf("User %s invited to %s\n", userEmail, channelName)
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
