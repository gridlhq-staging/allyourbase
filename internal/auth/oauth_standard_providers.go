// Package auth oauth_standard_providers.go defines OAuth2 userinfo parsers for
// standard platforms including Discord, Twitter, Facebook, LinkedIn, Spotify,
// Twitch, GitLab, Bitbucket, Slack, Zoom, Notion, and Figma. Each parser
// normalizes the provider's response format to a common OAuthUserInfo
// structure.
package auth

import (
	"encoding/json"
	"fmt"
	"strings"
)

// --- Discord ---

// parseDiscordUser unmarshals the Discord userinfo response and returns an OAuthUserInfo with the user ID, email, and name from the global_name field or username if global_name is empty. It returns an error if the user ID is missing.
func parseDiscordUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		Email      string `json:"email"`
		GlobalName string `json:"global_name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Discord user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	name := u.GlobalName
	if name == "" {
		name = u.Username
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           name,
	}, nil
}

// --- Twitter/X ---

// parseTwitterUser unmarshals the Twitter userinfo response (nested under the data field) and returns an OAuthUserInfo with the user ID, and name from the name field or username if name is empty. Email is not available from the Twitter API v2 users/me endpoint. It returns an error if the user ID is missing.
func parseTwitterUser(body []byte) (*OAuthUserInfo, error) {
	var resp struct {
		Data struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing Twitter user: %w", err)
	}
	if resp.Data.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	name := resp.Data.Name
	if name == "" {
		name = resp.Data.Username
	}
	return &OAuthUserInfo{
		ProviderUserID: resp.Data.ID,
		Name:           name,
		// Twitter API v2 doesn't return email in users/me.
	}, nil
}

// --- Facebook ---

// parseFacebookUser unmarshals the Facebook userinfo response and returns an OAuthUserInfo with the user ID, email, and name. It returns an error if the user ID is missing.
func parseFacebookUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Facebook user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           u.Name,
	}, nil
}

// --- LinkedIn ---

// parseLinkedInUser unmarshals the LinkedIn userinfo response and returns an OAuthUserInfo with the user ID (sub), email, and name. It returns an error if the user ID is missing.
func parseLinkedInUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing LinkedIn user: %w", err)
	}
	if u.Sub == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.Sub,
		Email:          u.Email,
		Name:           u.Name,
	}, nil
}

// --- Spotify ---

// parseSpotifyUser unmarshals the Spotify userinfo response and returns an OAuthUserInfo with the user ID, email, and display name. It returns an error if the user ID is missing.
func parseSpotifyUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Spotify user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           u.DisplayName,
	}, nil
}

// --- Twitch ---

// parseTwitchUser unmarshals the Twitch userinfo response (a data array) and returns an OAuthUserInfo from the first user entry with the user ID, email, and name from the display_name field or login if display_name is empty. It returns an error if the data array is empty or the user ID is missing.
func parseTwitchUser(body []byte) (*OAuthUserInfo, error) {
	var resp struct {
		Data []struct {
			ID          string `json:"id"`
			Login       string `json:"login"`
			DisplayName string `json:"display_name"`
			Email       string `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing Twitch user: %w", err)
	}
	if len(resp.Data) == 0 || resp.Data[0].ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	u := resp.Data[0]
	name := u.DisplayName
	if name == "" {
		name = u.Login
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           name,
	}, nil
}

// --- GitLab ---

// parseGitLabUser unmarshals the GitLab userinfo response and returns an OAuthUserInfo with the user ID (converted to string), email, and name from the name field or username if name is empty. It returns an error if the user ID is zero.
func parseGitLabUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing GitLab user: %w", err)
	}
	if u.ID == 0 {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	name := u.Name
	if name == "" {
		name = u.Username
	}
	return &OAuthUserInfo{
		ProviderUserID: fmt.Sprintf("%d", u.ID),
		Email:          u.Email,
		Name:           name,
	}, nil
}

// --- Bitbucket ---

// parseBitbucketUser unmarshals the Bitbucket userinfo response and returns an OAuthUserInfo with the user ID (UUID), and name from the display_name field or nickname if display_name is empty. Email must be fetched separately via the Bitbucket emails endpoint. It returns an error if the UUID is missing.
func parseBitbucketUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		UUID        string `json:"uuid"`
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Bitbucket user: %w", err)
	}
	if u.UUID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	name := u.DisplayName
	if name == "" {
		name = u.Nickname
	}
	return &OAuthUserInfo{
		ProviderUserID: u.UUID,
		Name:           name,
		// Bitbucket requires a separate email endpoint — handled in fetchUserInfoWithConfig.
	}, nil
}

// --- Slack ---

// parseSlackUser unmarshals the Slack userinfo response and returns an OAuthUserInfo with the user ID (sub), email, and name. It returns an error if the user ID is missing.
func parseSlackUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Slack user: %w", err)
	}
	if u.Sub == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.Sub,
		Email:          u.Email,
		Name:           u.Name,
	}, nil
}

// --- Zoom ---

// parseZoomUser unmarshals the Zoom userinfo response and returns an OAuthUserInfo with the user ID, email, and name from the display_name field or a concatenation of first_name and last_name if display_name is empty. It returns an error if the user ID is missing.
func parseZoomUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID          string `json:"id"`
		Email       string `json:"email"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Zoom user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	name := u.DisplayName
	if name == "" {
		name = strings.TrimSpace(u.FirstName + " " + u.LastName)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           name,
	}, nil
}

// --- Notion ---
// Notion returns user info in the token response body (owner.user),
// not via a separate userinfo endpoint.

// extractNotionUserFromTokenResponse unmarshals the Notion OAuth token response body and extracts user info from the owner.user field (Notion returns user info in the token response rather than via a separate userinfo endpoint). It returns an OAuthUserInfo with the user ID, email, and name. It returns an error if the owner type is not user or the user ID is missing.
func extractNotionUserFromTokenResponse(body []byte) (*OAuthUserInfo, error) {
	var resp struct {
		Owner struct {
			Type string `json:"type"`
			User struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Person struct {
					Email string `json:"email"`
				} `json:"person"`
			} `json:"user"`
		} `json:"owner"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing Notion token response: %w", err)
	}
	if resp.Owner.Type != "user" {
		return nil, fmt.Errorf("notion owner is not a user (type=%q)", resp.Owner.Type)
	}
	if resp.Owner.User.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: resp.Owner.User.ID,
		Email:          resp.Owner.User.Person.Email,
		Name:           resp.Owner.User.Name,
	}, nil
}

// --- Figma ---

// parseFigmaUser unmarshals the Figma userinfo response and returns an OAuthUserInfo with the user ID, email, and name from the handle field. It returns an error if the user ID is missing.
func parseFigmaUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID     string `json:"id"`
		Handle string `json:"handle"`
		Email  string `json:"email"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Figma user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           u.Handle,
	}, nil
}
