package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/allyourbase/ayb/internal/push"
	"github.com/spf13/cobra"
)

var pushListDeliveriesCmd = &cobra.Command{
	Use:   "list-deliveries",
	Short: "List push delivery history",
	RunE:  runPushListDeliveries,
}

func init() {
	pushListDeliveriesCmd.Flags().String("app-id", "", "Filter by app ID")
	pushListDeliveriesCmd.Flags().String("user-id", "", "Filter by user ID")
	pushListDeliveriesCmd.Flags().String("status", "", "Filter by status (pending|sent|failed|invalid_token)")
	pushListDeliveriesCmd.Flags().Int("limit", 50, "Maximum results")
	pushListDeliveriesCmd.Flags().Int("offset", 0, "Offset")
}

// runPushListDeliveries lists push delivery history with optional filtering by app-id, user-id, and status, supporting pagination via limit and offset parameters. Results are output in table, JSON, or CSV format.
func runPushListDeliveries(cmd *cobra.Command, _ []string) error {
	outFmt := outputFormat(cmd)
	appID, _ := cmd.Flags().GetString("app-id")
	userID, _ := cmd.Flags().GetString("user-id")
	status, _ := cmd.Flags().GetString("status")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	if status != "" {
		switch status {
		case push.DeliveryStatusPending, push.DeliveryStatusSent, push.DeliveryStatusFailed, push.DeliveryStatusInvalidToken:
		default:
			return fmt.Errorf("--status must be one of: pending, sent, failed, invalid_token")
		}
	}
	if limit <= 0 {
		return fmt.Errorf("--limit must be greater than 0")
	}
	if offset < 0 {
		return fmt.Errorf("--offset must be greater than or equal to 0")
	}

	q := url.Values{}
	if appID != "" {
		q.Set("app_id", appID)
	}
	if userID != "" {
		q.Set("user_id", userID)
	}
	if status != "" {
		q.Set("status", status)
	}
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("offset", fmt.Sprintf("%d", offset))

	resp, body, err := adminRequest(cmd, http.MethodGet, "/api/admin/push/deliveries?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	var result struct {
		Items []struct {
			ID        string  `json:"id"`
			Provider  string  `json:"provider"`
			Title     string  `json:"title"`
			Status    string  `json:"status"`
			ErrorCode *string `json:"error_code"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if outFmt == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Items)
	}
	if len(result.Items) == 0 {
		fmt.Println("No push deliveries found.")
		return nil
	}
	if outFmt == "csv" {
		rows := make([][]string, 0, len(result.Items))
		for _, item := range result.Items {
			errorCode := ""
			if item.ErrorCode != nil {
				errorCode = *item.ErrorCode
			}
			rows = append(rows, []string{
				item.ID,
				item.Provider,
				item.Status,
				item.Title,
				errorCode,
			})
		}
		return writeCSVStdout([]string{"ID", "Provider", "Status", "Title", "Error"}, rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPROVIDER\tSTATUS\tTITLE\tERROR")
	for _, item := range result.Items {
		errorCode := "-"
		if item.ErrorCode != nil && strings.TrimSpace(*item.ErrorCode) != "" {
			errorCode = *item.ErrorCode
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", item.ID, item.Provider, item.Status, item.Title, errorCode)
	}
	return w.Flush()
}
