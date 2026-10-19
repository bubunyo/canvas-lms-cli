package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bubunyo/canvas-lms-cli/internal/canvas"
)

var coursesAll bool

var coursesCmd = &cobra.Command{
	Use:   "courses",
	Short: "List your courses",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cl, err := client()
		if err != nil {
			return err
		}
		q := url.Values{"include[]": {"term"}}
		if !coursesAll {
			q.Set("enrollment_state", "active")
		}
		courses, err := canvas.GetAll[canvas.Course](cmd.Context(), cl, "/api/v1/courses", q)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(courses)
		}
		w := table()
		fmt.Fprintln(w, "ID\tCODE\tNAME\tTERM")
		for _, c := range courses {
			term := "-"
			if c.Term != nil {
				term = c.Term.Name
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.ID, c.CourseCode, c.Name, term)
		}
		return w.Flush()
	},
}

var modulesCmd = &cobra.Command{
	Use:   "modules <course>",
	Short: "List a course's modules and items (course = id, code, or name fragment)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cl, err := client()
		if err != nil {
			return err
		}
		id, err := resolveCourse(ctx, cl, args[0])
		if err != nil {
			return err
		}
		mods, err := courseModules(ctx, cl, id)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(mods)
		}
		for _, m := range mods {
			done, req := 0, 0
			for _, it := range m.Items {
				if it.CompletionRequirement != nil {
					req++
					if it.CompletionRequirement.Completed {
						done++
					}
				}
			}
			progress := ""
			if req > 0 {
				progress = fmt.Sprintf("  [%d/%d]", done, req)
			}
			fmt.Printf("\n%s%s\n", m.Name, progress)
			for _, it := range m.Items {
				mark := " "
				if r := it.CompletionRequirement; r != nil {
					mark = "○"
					if r.Completed {
						mark = "✓"
					}
				}
				if it.Type == "SubHeader" {
					fmt.Printf("  %s── %s\n", strings.Repeat("  ", it.Indent), it.Title)
					continue
				}
				fmt.Printf("  %s %s%s  (%s)\n", mark, strings.Repeat("  ", it.Indent), it.Title, it.Type)
			}
		}
		return nil
	},
}

var todoDays int

var todoCmd = &cobra.Command{
	Use:   "todo",
	Short: "Upcoming planner items (assignments, quizzes, discussions, events)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cl, err := client()
		if err != nil {
			return err
		}
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		q := url.Values{
			"start_date": {start.UTC().Format(time.RFC3339)},
			"end_date":   {start.AddDate(0, 0, todoDays).UTC().Format(time.RFC3339)},
			"order":      {"asc"},
		}
		items, err := canvas.GetAll[canvas.PlannerItem](cmd.Context(), cl, "/api/v1/planner/items", q)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(items)
		}
		if len(items) == 0 {
			fmt.Printf("Nothing in the next %d days.\n", todoDays)
			return nil
		}
		w := table()
		fmt.Fprintln(w, "WHEN\tCOURSE\tTYPE\tTITLE\tSTATUS")
		for _, it := range items {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", fmtTime(&it.PlannableDate), it.ContextName, it.PlannableType, it.Plannable.Title, plannerStatus(it))
		}
		return w.Flush()
	},
}

func plannerStatus(it canvas.PlannerItem) string {
	if it.PlannerOverride != nil && it.PlannerOverride.MarkedComplete {
		return "done"
	}
	s := it.Submission()
	switch {
	case s == nil:
		if it.NewActivity {
			return "new"
		}
		return ""
	case s.Excused:
		return "excused"
	case s.Graded:
		return "graded"
	case s.Submitted && s.Late:
		return "submitted (late)"
	case s.Submitted:
		return "submitted"
	case s.Missing:
		return "MISSING"
	}
	return "to do"
}

var missingCmd = &cobra.Command{
	Use:   "missing",
	Short: "Past-due assignments you haven't submitted",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cl, err := client()
		if err != nil {
			return err
		}
		q := url.Values{"include[]": {"course"}, "filter[]": {"submittable", "current_grading_period"}}
		as, err := canvas.GetAll[canvas.Assignment](cmd.Context(), cl, "/api/v1/users/self/missing_submissions", q)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(as)
		}
		if len(as) == 0 {
			fmt.Println("No missing submissions.")
			return nil
		}
		w := table()
		fmt.Fprintln(w, "DUE\tCOURSE\tASSIGNMENT\tURL")
		for _, a := range as {
			course := string(a.CourseID)
			if a.Course != nil {
				course = a.Course.CourseCode
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", fmtTime(a.DueAt), course, a.Name, a.HTMLURL)
		}
		return w.Flush()
	},
}

var apiCmd = &cobra.Command{
	Use:   "api <path>",
	Short: "Authenticated GET against any API path, e.g. `canvas-cli api /api/v1/users/self/profile`",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cl, err := client()
		if err != nil {
			return err
		}
		path := args[0]
		if !strings.HasPrefix(path, "/") {
			path = "/api/v1/" + path
		}
		var raw json.RawMessage
		if err := cl.Get(cmd.Context(), path, nil, &raw); err != nil {
			return err
		}
		var v any
		if json.Unmarshal(raw, &v) != nil {
			fmt.Println(string(raw))
			return nil
		}
		return printJSON(v)
	},
}

func init() {
	coursesCmd.Flags().BoolVar(&coursesAll, "all", false, "include completed/inactive enrollments")
	todoCmd.Flags().IntVar(&todoDays, "days", 14, "how many days ahead to look")
	rootCmd.AddCommand(coursesCmd, modulesCmd, todoCmd, missingCmd, apiCmd)
}
