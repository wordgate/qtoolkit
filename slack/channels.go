package slack

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// ErrChannelNameTaken is wrapped by CreateChannel when Slack answers
// "name_taken". The returned error also wraps ErrAPIFailed.
var ErrChannelNameTaken = errors.New("slack: channel name already taken")

const (
	// maxInviteUsers is the most user ids conversations.invite accepts per call.
	maxInviteUsers = 1000
	// maxTopicLength is Slack's limit for a channel topic, in characters.
	maxTopicLength = 250
	// membersPageSize is the page size requested from conversations.members.
	membersPageSize = 200
)

// CreateChannel creates a channel (conversations.create) and returns its id.
// name must already satisfy Slack's rules (lowercase, at most 80 characters,
// no spaces or periods); it is sent as is. A name that is already in use
// yields an error wrapping both ErrChannelNameTaken and ErrAPIFailed.
// Requires bot_token with the channels:manage scope (groups:write if private).
func CreateChannel(ctx context.Context, name string, private bool) (channelID string, err error) {
	payload := map[string]any{
		"name":       name,
		"is_private": private,
	}
	var result struct {
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
	}
	if err := callAPI(ctx, "conversations.create", nil, payload, &result); err != nil {
		if slackErrorCode(err) == "name_taken" {
			return "", fmt.Errorf("%w: %w", ErrChannelNameTaken, err)
		}
		return "", err
	}
	if result.Channel.ID == "" {
		return "", fmt.Errorf("%w: conversations.create: response has no channel id", ErrAPIFailed)
	}
	return result.Channel.ID, nil
}

// InviteToChannel adds users to a channel (conversations.invite). More than
// 1000 user ids are sent as consecutive calls of at most 1000 each; the first
// failing call aborts the rest. Slack's "already_in_channel" counts as
// success. With no user ids it returns nil without calling Slack.
// Requires bot_token with the channels:write.invites scope
// (groups:write.invites for private channels); the bot must be a member.
func InviteToChannel(ctx context.Context, channelID string, userIDs []string) error {
	for start := 0; start < len(userIDs); start += maxInviteUsers {
		batch := userIDs[start:min(start+maxInviteUsers, len(userIDs))]
		payload := map[string]any{
			"channel": channelID,
			"users":   strings.Join(batch, ","),
		}
		err := callAPI(ctx, "conversations.invite", nil, payload, nil)
		if err != nil && slackErrorCode(err) != "already_in_channel" {
			return err
		}
	}
	return nil
}

// ArchiveChannel archives a channel (conversations.archive). Slack's
// "already_archived" counts as success.
// Requires bot_token with the channels:manage scope (groups:write if private).
func ArchiveChannel(ctx context.Context, channelID string) error {
	err := callAPI(ctx, "conversations.archive", nil, map[string]any{"channel": channelID}, nil)
	if err != nil && slackErrorCode(err) != "already_archived" {
		return err
	}
	return nil
}

// ChannelMembers returns the user ids of all members of a channel
// (conversations.members), following response_metadata.next_cursor until the
// list is complete. If any page fails, the error is returned and no partial
// list.
// Requires bot_token with the channels:read scope (groups:read if private).
func ChannelMembers(ctx context.Context, channelID string) ([]string, error) {
	var members []string
	cursor := ""
	for {
		query := url.Values{
			"channel": {channelID},
			"limit":   {strconv.Itoa(membersPageSize)},
		}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var page struct {
			Members          []string `json:"members"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := callAPI(ctx, "conversations.members", query, nil, &page); err != nil {
			return nil, err
		}
		members = append(members, page.Members...)

		next := page.ResponseMetadata.NextCursor
		if next == "" {
			return members, nil
		}
		if next == cursor {
			return nil, fmt.Errorf("%w: conversations.members: cursor did not advance", ErrAPIFailed)
		}
		cursor = next
	}
}

// SetChannelTopic sets a channel's topic (conversations.setTopic). A topic
// longer than 250 characters (runes) is cut to its first 250.
// Requires bot_token with the channels:manage scope (groups:write if private).
func SetChannelTopic(ctx context.Context, channelID, topic string) error {
	if runes := []rune(topic); len(runes) > maxTopicLength {
		topic = string(runes[:maxTopicLength])
	}
	payload := map[string]any{
		"channel": channelID,
		"topic":   topic,
	}
	return callAPI(ctx, "conversations.setTopic", nil, payload, nil)
}

// botUser caches the result of auth.test for one bot token.
var botUser struct {
	sync.RWMutex
	token string
	id    string
}

// resetBotUserID drops the cached BotUserID result (for tests).
func resetBotUserID() {
	botUser.Lock()
	defer botUser.Unlock()
	botUser.token, botUser.id = "", ""
}

// BotUserID returns the bot's own user id (auth.test → user_id).
//
// A successful result is cached for the life of the process, per bot token:
// changing the token with SetConfig triggers a fresh lookup. Failures are not
// cached. It is safe for concurrent use; callers racing on a cold cache may
// each call auth.test once.
func BotUserID(ctx context.Context) (string, error) {
	token := getConfig().BotToken
	if token == "" {
		return "", ErrNoBotToken
	}

	botUser.RLock()
	id, hit := botUser.id, botUser.token == token
	botUser.RUnlock()
	if hit && id != "" {
		return id, nil
	}

	var result struct {
		UserID string `json:"user_id"`
	}
	if err := callAPI(ctx, "auth.test", nil, map[string]any{}, &result); err != nil {
		return "", err
	}
	if result.UserID == "" {
		return "", fmt.Errorf("%w: auth.test: response has no user_id", ErrAPIFailed)
	}

	botUser.Lock()
	botUser.token, botUser.id = token, result.UserID
	botUser.Unlock()
	return result.UserID, nil
}
