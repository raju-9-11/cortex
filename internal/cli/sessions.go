package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"forge/internal/session"
)

// RunSessions handles the `forge sessions` subcommand.
// action: "" or "list" → list sessions
//
//	"show"   → show session details
//	"delete" → delete session
func RunSessions(ctx context.Context, sessionMgr session.Manager,
	action string, targetID string, limit int, jsonOutput bool, w io.Writer) error {

	switch action {
	case "", "list":
		return listSessions(ctx, sessionMgr, limit, jsonOutput, w)
	case "show":
		return showSession(ctx, sessionMgr, targetID, jsonOutput, w)
	case "delete":
		return deleteSession(ctx, sessionMgr, targetID, w)
	default:
		return fmt.Errorf("unknown sessions action %q. Use: list, show, delete", action)
	}
}

func listSessions(ctx context.Context, mgr session.Manager, limit int, jsonOut bool, w io.Writer) error {
	sessions, err := mgr.List(ctx, "default")
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Fprintln(w, "No sessions found.")
		return nil
	}

	// Apply limit
	if limit > 0 && limit < len(sessions) {
		sessions = sessions[:limit]
	}

	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(sessions)
	}

	// Table output using tabwriter
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tMODEL\tMESSAGES\tLAST ACCESS")
	for _, s := range sessions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
			s.ID,
			truncate(s.Title, 30),
			s.Model,
			s.MessageCount,
			formatTimeAgo(s.LastAccess),
		)
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%d session(s)\n", len(sessions))
	return nil
}

func showSession(ctx context.Context, mgr session.Manager, id string, jsonOut bool, w io.Writer) error {
	sess, err := mgr.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("session %q not found", id)
	}

	messages, err := mgr.GetMessages(ctx, id)
	if err != nil {
		return err
	}

	if jsonOut {
		// JSON output with session + messages
		result := struct {
			Session  interface{} `json:"session"`
			Messages interface{} `json:"messages"`
		}{sess, messages}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Human-readable output
	fmt.Fprintf(w, "Session: %s\n", sess.ID)
	fmt.Fprintf(w, "Title:   %s\n", sess.Title)
	fmt.Fprintf(w, "Model:   %s\n", sess.Model)
	fmt.Fprintf(w, "Status:  %s\n", sess.Status)
	fmt.Fprintf(w, "Created: %s\n", sess.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "Messages: %d\n\n", sess.MessageCount)

	for _, msg := range messages {
		fmt.Fprintf(w, "[%s] %s\n\n", msg.Role, msg.Content)
	}

	return nil
}

func deleteSession(ctx context.Context, mgr session.Manager, id string, w io.Writer) error {
	if err := mgr.Delete(ctx, id); err != nil {
		return fmt.Errorf("session %q not found", id)
	}
	fmt.Fprintf(w, "Session %s deleted.\n", id)
	return nil
}

// truncate shortens a string with ellipsis if it exceeds maxLen.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// formatTimeAgo formats a time as a relative duration like "2h ago", "3d ago".
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
