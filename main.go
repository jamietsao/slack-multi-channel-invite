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
	conversationsKickURL   = "https://slack.com/api/conversations.kick"
	conversationsListURL   = "https://slack.com/api/conversations.list"
	usersLookupByEmailURL  = "https://slack.com/api/users.lookupByEmail"

	actionAdd    = "add"
	actionRemove = "remove"
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

	conversationsKickRequest struct {
		ChannelID string `json:"channel"`
		UserID    string `json:"user"`
	}

	conversationsKickResponse struct {
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

// This script invites the given users to the given channels on Slack.
// Due to the oddness of the Slack API, this is accomplished via these steps:
// 1) Look up Slack user IDs by email
// 2) Query all public (private if 'private' flag is set to true) channels in the workspace and create a name -> ID mapping
// 3) For each of the given channels, invite the users (user IDs) to the channel (channel ID)
func main() {
	var apiToken string
	var action string
	var emails string
	var channelsArg string
	var private bool
	var debug bool

	// parse flags
	flag.StringVar(&apiToken, "api_token", "", "Slack OAuth Access Token")
	flag.StringVar(&action, "action", "add", "'add' to invite users, 'remove' to remove users")
	flag.StringVar(&emails, "emails", "", "Comma separated list of Slack user emails to invite")
	flag.StringVar(&channelsArg, "channels", "", "Comma separated list of channels to invite users to")
	flag.BoolVar(&private, "private", false, "Boolean flag to enable private channel invitations (requires OAuth scopes 'groups:read' and 'groups:write')")
	flag.BoolVar(&debug, "debug", false, "Enables debug logging when set to true")
	flag.Parse()

	if apiToken == "" || emails == "" || channelsArg == "" || (action != actionAdd && action != actionRemove) {
		flag.Usage()
		os.Exit(1)
	}

	// lookup users by email
	fmt.Printf("\nLooking up users ...\n")
	var userIDs []string
	for _, email := range strings.Split(emails, ",") {
		userID, err := getUserID(apiToken, email)
		if err != nil {
			fmt.Printf("Error while looking up user with email %s: %s\n", email, err)
			continue
		}

		fmt.Printf("Valid user (ID: %s) found for '%s'\n", userID, email)
		userIDs = append(userIDs, userID)
	}

	if len(userIDs) == 0 {
		fmt.Println("\nNo users found - aborting")
		return
	}

	// get all channels
	channelNameToIDMap, err := getChannels(apiToken, private, debug)
	if err != nil {
		panic(err)
	}

	if debug {
		fmt.Printf("DEBUG: Total # of channels retrieved: %d\n", len(channelNameToIDMap))
	}

	// invite/remove users to each channel
	if action == actionAdd {
		fmt.Printf("\nInviting users to channels ...\n")
	} else {
		fmt.Printf("\nRemoving users from channels ...\n")
	}
	channels := strings.Split(channelsArg, ",")
	for _, channel := range channels {
		channelID := channelNameToIDMap[channel]
		if channelID == "" {
			fmt.Printf("Channel '%s' not found -- skipping\n", channel)
			continue
		}

		if action == actionAdd {
			err := inviteUsersToChannel(apiToken, userIDs, channelID)
			if err != nil {
				fmt.Printf("Error while inviting users to %s (%s): %s\n", channel, channelID, err)
				continue
			}
		} else {
			err := removeUsersFromChannel(apiToken, userIDs, channelID, debug)
			if err != nil {
				fmt.Printf("Error while removing users from %s (%s): %s\n", channel, channelID, err)
				continue
			}
		}

		if action == actionAdd {
			fmt.Printf("Users invited to '%s'\n", channel)
		} else {
			fmt.Printf("Users removed from '%s'\n", channel)
		}
	}

	fmt.Println("\nAll done! You're welcome =)")
}

func getUserID(apiToken, userEmail string) (string, error) {
	httpClient := &http.Client{}

	// lookup user by email
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(usersLookupByEmailURL+"?email=%s", userEmail), nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
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

func getChannels(apiToken string, private bool, debug bool) (map[string]string, error) {

	channelType := "public_channel"
	if private {
		channelType = "private_channel"
	}

	nameToID := make(map[string]string)

	httpClient := &http.Client{}
	var nextCursor string
	for {
		// query list of channels
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(conversationsListURL+"?cursor=%s&exclude_archived=true&limit=200&types=%s", nextCursor, channelType), nil)
		if err != nil {
			return nil, err
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
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

		if debug {
			fmt.Printf("DEBUG: # of channels returned in page: %d\n", len(data.Channels))
		}

		// map of channel names to IDs
		for _, channel := range data.Channels {
			nameToID[channel.Name] = channel.ID
		}

		// paginate if necessary
		nextCursor = data.ResponseMetadata.NextCursor
		if nextCursor == "" {
			break
		}
	}

	return nameToID, nil
}

func inviteUsersToChannel(apiToken string, userIDs []string, channelID string) error {
	httpClient := &http.Client{}

	reqBody, err := json.Marshal(conversationsInviteRequest{
		ChannelID: channelID,
		UserIDs:   strings.Join(userIDs, ","),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, conversationsInviteURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := httpClient.Do(req)
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

func removeUsersFromChannel(apiToken string, userIDs []string, channelID string, debug bool) error {
	// API only supports removing users one at a time ...
	for _, userID := range userIDs {
		err := removeUserFromChannel(apiToken, userID, channelID)
		if err != nil {
			if debug {
				fmt.Printf("DEBUG: Error while removing user %s from channel %s: %s\n", userID, channelID, err)
			}
			return err
		}
	}
	return nil
}

func removeUserFromChannel(apiToken string, userID string, channelID string) error {
	httpClient := &http.Client{}

	reqBody, err := json.Marshal(conversationsKickRequest{
		ChannelID: channelID,
		UserID:    userID,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, conversationsKickURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := httpClient.Do(req)
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

	var data conversationsKickResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return err
	}

	if !data.Ok {
		fmt.Printf("conversationsKickResponse: %+v\n", data)
		return fmt.Errorf("Non-ok response while removing user from channel")
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
