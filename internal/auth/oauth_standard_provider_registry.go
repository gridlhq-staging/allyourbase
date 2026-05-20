package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Standard OAuth2 provider configs and userinfo parsers.
// Each provider follows the pattern: register in oauthProviders + oauthUserInfoParsers,
// implement a parseXxxUser function that normalizes to OAuthUserInfo.
func init() {
	registerStandardOAuthProviders()
}

func registerStandardOAuthProviders() {
	oauthMu.Lock()
	defer oauthMu.Unlock()

	registerOAuthProviderConfigsLocked(oauthStandardProviderConfigs())
	registerOAuthUserInfoParsersLocked(oauthStandardUserInfoParsers())

	notion := notionOAuthProviderConfig()
	oauthProviders["notion"] = notion
	defaultProviders["notion"] = notion
}

func registerOAuthProviderConfigsLocked(providers map[string]OAuthProviderConfig) {
	for name, cfg := range providers {
		oauthProviders[name] = cfg
		defaultProviders[name] = cfg
	}
}

func registerOAuthUserInfoParsersLocked(parsers map[string]OAuthUserInfoParser) {
	for name, parser := range parsers {
		oauthUserInfoParsers[name] = parser
		defaultUserInfoParsers[name] = parser
	}
}

func oauthStandardProviderConfigs() map[string]OAuthProviderConfig {
	return map[string]OAuthProviderConfig{
		"discord": {
			AuthURL:     "https://discord.com/api/oauth2/authorize",
			TokenURL:    "https://discord.com/api/oauth2/token",
			UserInfoURL: "https://discord.com/api/users/@me",
			Scopes:      []string{"identify", "email"},
		},
		"twitter": {
			AuthURL:         "https://twitter.com/i/oauth2/authorize",
			TokenURL:        "https://api.twitter.com/2/oauth2/token",
			UserInfoURL:     "https://api.twitter.com/2/users/me?user.fields=profile_image_url",
			Scopes:          []string{"users.read", "tweet.read", "offline.access"},
			TokenAuthMethod: OAuthTokenAuthMethodClientSecretBasic,
		},
		"facebook": {
			AuthURL:     "https://www.facebook.com/v22.0/dialog/oauth",
			TokenURL:    "https://graph.facebook.com/v22.0/oauth/access_token",
			UserInfoURL: "https://graph.facebook.com/v22.0/me?fields=id,name,email,picture",
			Scopes:      []string{"email", "public_profile"},
		},
		"linkedin": {
			AuthURL:     "https://www.linkedin.com/oauth/v2/authorization",
			TokenURL:    "https://www.linkedin.com/oauth/v2/accessToken",
			UserInfoURL: "https://api.linkedin.com/v2/userinfo",
			Scopes:      []string{"openid", "profile", "email"},
		},
		"spotify": {
			AuthURL:     "https://accounts.spotify.com/authorize",
			TokenURL:    "https://accounts.spotify.com/api/token",
			UserInfoURL: "https://api.spotify.com/v1/me",
			Scopes:      []string{"user-read-email", "user-read-private"},
		},
		"twitch": {
			AuthURL:     "https://id.twitch.tv/oauth2/authorize",
			TokenURL:    "https://id.twitch.tv/oauth2/token",
			UserInfoURL: "https://api.twitch.tv/helix/users",
			Scopes:      []string{"user:read:email"},
			UserInfoHeaders: map[string]string{
				"Client-Id": "{client_id}",
			},
		},
		"gitlab": {
			AuthURL:     "https://gitlab.com/oauth/authorize",
			TokenURL:    "https://gitlab.com/oauth/token",
			UserInfoURL: "https://gitlab.com/api/v4/user",
			Scopes:      []string{"read_user"},
		},
		"bitbucket": {
			AuthURL:     "https://bitbucket.org/site/oauth2/authorize",
			TokenURL:    "https://bitbucket.org/site/oauth2/access_token",
			UserInfoURL: "https://api.bitbucket.org/2.0/user",
			Scopes:      []string{"account", "email"},
		},
		"slack": {
			AuthURL:     "https://slack.com/openid/connect/authorize",
			TokenURL:    "https://slack.com/api/openid.connect.token",
			UserInfoURL: "https://slack.com/api/openid.connect.userInfo",
			Scopes:      []string{"openid", "profile", "email"},
		},
		"zoom": {
			AuthURL:         "https://zoom.us/oauth/authorize",
			TokenURL:        "https://zoom.us/oauth/token",
			UserInfoURL:     "https://api.zoom.us/v2/users/me",
			Scopes:          []string{"user:read:email"},
			TokenAuthMethod: OAuthTokenAuthMethodClientSecretBasic,
		},
		"figma": {
			AuthURL:     "https://www.figma.com/oauth",
			TokenURL:    "https://api.figma.com/v1/oauth/token",
			UserInfoURL: "https://api.figma.com/v1/me",
			Scopes:      []string{"file_read"},
		},
	}
}

func oauthStandardUserInfoParsers() map[string]OAuthUserInfoParser {
	return map[string]OAuthUserInfoParser{
		"discord":   parseDiscordUser,
		"twitter":   parseTwitterUser,
		"facebook":  parseFacebookUser,
		"linkedin":  parseLinkedInUser,
		"spotify":   parseSpotifyUser,
		"twitch":    parseTwitchUser,
		"gitlab":    parseGitLabUser,
		"bitbucket": parseBitbucketUser,
		"slack":     parseSlackUser,
		"zoom":      parseZoomUser,
		"figma":     parseFigmaUser,
	}
}

func notionOAuthProviderConfig() OAuthProviderConfig {
	return OAuthProviderConfig{
		AuthURL:                        "https://api.notion.com/v1/oauth/authorize",
		TokenURL:                       "https://api.notion.com/v1/oauth/token",
		Scopes:                         []string{},
		TokenAuthMethod:                OAuthTokenAuthMethodClientSecretBasic,
		UserInfoSource:                 OAuthUserInfoSourceTokenResponse,
		TokenResponseUserInfoExtractor: extractNotionUserFromTokenResponse,
	}
}

// SetFacebookAPIVersion updates the Facebook provider URLs to use the given
// Graph API version (e.g., "v22.0"). Called at startup if the operator sets
// auth.oauth.facebook.facebook_api_version in ayb.toml.
func SetFacebookAPIVersion(version string) {
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	oauthMu.Lock()
	defer oauthMu.Unlock()

	cfg, ok := oauthProviders["facebook"]
	if !ok {
		return
	}
	cfg.AuthURL = "https://www.facebook.com/" + version + "/dialog/oauth"
	cfg.TokenURL = "https://graph.facebook.com/" + version + "/oauth/access_token"
	cfg.UserInfoURL = "https://graph.facebook.com/" + version + "/me?fields=id,name,email,picture"
	oauthProviders["facebook"] = cfg
}

// SetGitLabBaseURL updates the GitLab provider URLs to use the given base URL
// (e.g., "https://gitlab.example.com" for a self-hosted instance). Called at
// startup if the operator sets auth.oauth.gitlab.gitlab_base_url in ayb.toml.
func SetGitLabBaseURL(baseURL string) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return
	}
	oauthMu.Lock()
	defer oauthMu.Unlock()

	cfg, ok := oauthProviders["gitlab"]
	if !ok {
		return
	}
	cfg.AuthURL = baseURL + "/oauth/authorize"
	cfg.TokenURL = baseURL + "/oauth/token"
	cfg.UserInfoURL = baseURL + "/api/v4/user"
	oauthProviders["gitlab"] = cfg
}

// fetchBitbucketPrimaryEmail fetches the primary email from the Bitbucket
// emails endpoint (/2.0/user/emails). Similar to fetchGitHubPrimaryEmail.
func fetchBitbucketPrimaryEmail(ctx context.Context, accessToken string, httpClient *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bitbucket.org/2.0/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bitbucket emails endpoint returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var emailResp struct {
		Values []struct {
			Email       string `json:"email"`
			IsPrimary   bool   `json:"is_primary"`
			IsConfirmed bool   `json:"is_confirmed"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &emailResp); err != nil {
		return "", err
	}

	for _, e := range emailResp.Values {
		if e.IsPrimary && e.IsConfirmed {
			return e.Email, nil
		}
	}
	for _, e := range emailResp.Values {
		if e.IsConfirmed {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no confirmed email found")
}
