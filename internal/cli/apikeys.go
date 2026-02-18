package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var apikeysCmd = &cobra.Command{
	Use:   "apikeys",
	Short: "Manage API keys on the running AYB server",
}

var apikeysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all API keys",
	RunE:  runAPIKeysList,
}

var apikeysCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new API key for a user",
	RunE:  runAPIKeysCreate,
}

var apikeysRevokeCmd = &cobra.Command{
	Use:   "revoke <id>",
	Short: "Revoke an API key",
	Args:  cobra.ExactArgs(1),
	RunE:  runAPIKeysRevoke,
}

func init() {
	apikeysCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	apikeysCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	apikeysCreateCmd.Flags().String("user-id", "", "User ID to create key for (required)")
	apikeysCreateCmd.Flags().String("name", "", "Key name/description (required)")
	apikeysCreateCmd.Flags().String("scope", "*", "Permission scope: * (full), readonly, readwrite")
	apikeysCreateCmd.Flags().StringSlice("tables", nil, "Restrict access to specific tables (comma-separated)")

	apikeysCmd.AddCommand(apikeysListCmd)
	apikeysCmd.AddCommand(apikeysCreateCmd)
	apikeysCmd.AddCommand(apikeysRevokeCmd)
}

func apikeysAdminRequest(cmd *cobra.Command, method, path string, body io.Reader) (*http.Response, []byte, error) {
	token, _ := cmd.Flags().GetString("admin-token")
	baseURL, _ := cmd.Flags().GetString("url")

	if token == "" {
		token = os.Getenv("AYB_ADMIN_TOKEN")
	}
	if baseURL == "" {
		baseURL = serverURL()
	}

	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to server: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp, respBody, nil
}

func runAPIKeysList(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)

	resp, body, err := apikeysAdminRequest(cmd, "GET", "/api/admin/api-keys", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	if outFmt == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}

	var result struct {
		Items []struct {
			ID         string   `json:"id"`
			UserID     string   `json:"userId"`
			Name       string   `json:"name"`
			KeyPrefix  string   `json:"keyPrefix"`
			Scope      string   `json:"scope"`
			AllowedTables []string `json:"allowedTables"`
			LastUsedAt *string  `json:"lastUsedAt"`
			CreatedAt  string   `json:"createdAt"`
			RevokedAt  *string  `json:"revokedAt"`
		} `json:"items"`
		TotalItems int `json:"totalItems"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Items) == 0 {
		fmt.Println("No API keys configured.")
		return nil
	}

	cols := []string{"ID", "User ID", "Name", "Key Prefix", "Scope", "Last Used", "Created", "Status"}
	rows := make([][]string, len(result.Items))
	for i, k := range result.Items {
		lastUsed := "never"
		if k.LastUsedAt != nil {
			lastUsed = *k.LastUsedAt
		}
		status := "active"
		if k.RevokedAt != nil {
			status = "revoked"
		}
		scope := k.Scope
		if len(k.AllowedTables) > 0 {
			scope += " [" + strings.Join(k.AllowedTables, ",") + "]"
		}
		rows[i] = []string{k.ID, k.UserID, k.Name, k.KeyPrefix + "...", scope, lastUsed, k.CreatedAt, status}
	}

	if outFmt == "csv" {
		return writeCSVStdout(cols, rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(cols, "\t"))
	fmt.Fprintln(w, strings.Repeat("---\t", len(cols)))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
	fmt.Printf("\n%d api key(s)\n", result.TotalItems)
	return nil
}

func runAPIKeysCreate(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)
	userID, _ := cmd.Flags().GetString("user-id")
	name, _ := cmd.Flags().GetString("name")
	scope, _ := cmd.Flags().GetString("scope")
	tables, _ := cmd.Flags().GetStringSlice("tables")

	if userID == "" {
		return fmt.Errorf("--user-id is required")
	}
	if name == "" {
		return fmt.Errorf("--name is required")
	}

	payload := map[string]any{
		"userId": userID,
		"name":   name,
		"scope":  scope,
	}
	if len(tables) > 0 {
		payload["allowedTables"] = tables
	}
	body, _ := json.Marshal(payload)

	resp, respBody, err := apikeysAdminRequest(cmd, "POST", "/api/admin/api-keys", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return serverError(resp.StatusCode, respBody)
	}

	if outFmt == "json" {
		os.Stdout.Write(respBody)
		fmt.Println()
		return nil
	}

	var result struct {
		Key    string `json:"key"`
		APIKey struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Scope string `json:"scope"`
		} `json:"apiKey"`
	}
	json.Unmarshal(respBody, &result)
	fmt.Printf("API key created: %s (%s)\n", result.APIKey.ID, result.APIKey.Name)
	fmt.Printf("Scope: %s\n", result.APIKey.Scope)
	if len(tables) > 0 {
		fmt.Printf("Tables: %s\n", strings.Join(tables, ", "))
	}
	fmt.Printf("\nKey: %s\n", result.Key)
	fmt.Println("\nSave this key — it will not be shown again.")
	return nil
}

func runAPIKeysRevoke(cmd *cobra.Command, args []string) error {
	id := args[0]

	resp, body, err := apikeysAdminRequest(cmd, "DELETE", "/api/admin/api-keys/"+id, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("API key %s revoked.\n", id)
		return nil
	}
	return serverError(resp.StatusCode, body)
}
