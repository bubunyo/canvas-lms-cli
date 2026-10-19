package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/bubunyo/canvas-lms-cli/internal/canvas"
	"github.com/bubunyo/canvas-lms-cli/internal/config"
	"github.com/bubunyo/canvas-lms-cli/internal/creds"
)

// Version is set at build time: -ldflags "-X github.com/bubunyo/canvas-lms-cli/internal/cli.Version=v1.2.3".
var Version = "dev"

var (
	cfg        *config.Config
	jsonOut    bool
	baseURLArg string
)

var rootCmd = &cobra.Command{
	Use:           "canvas-cli",
	Short:         "Command-line client for Canvas LMS",
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		var err error
		if cfg, err = config.Load(); err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if baseURLArg != "" {
			cfg.BaseURL = strings.TrimRight(baseURLArg, "/")
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "print raw JSON")
	rootCmd.PersistentFlags().StringVar(&baseURLArg, "base-url", "", "Canvas instance URL (default from config, CANVAS_BASE_URL, or "+config.DefaultBaseURL+")")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func client() (*canvas.Client, error) {
	c, err := creds.Load(cfg.BaseURL)
	if errors.Is(err, creds.ErrNotFound) {
		return nil, fmt.Errorf("not logged in to %s: run `canvas-cli auth login`", cfg.BaseURL)
	}
	if err != nil {
		return nil, fmt.Errorf("reading keychain: %w", err)
	}
	return canvas.New(cfg.BaseURL, c), nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func table() *tabwriter.Writer { return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0) }

func fmtTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.Local().Format("Mon 02 Jan 15:04")
}

// resolveCourse accepts a numeric course id, a course code (COM00150M) or a
// unique case-insensitive substring of the course name.
func resolveCourse(ctx context.Context, cl *canvas.Client, arg string) (canvas.ID, error) {
	if strings.Trim(arg, "0123456789") == "" {
		return canvas.ID(arg), nil
	}
	courses, err := canvas.GetAll[canvas.Course](ctx, cl, "/api/v1/users/self/courses", url.Values{})
	if err != nil {
		return "", err
	}
	needle := strings.ToLower(arg)
	var matches []canvas.Course
	for _, c := range courses {
		if strings.EqualFold(c.CourseCode, arg) {
			return c.ID, nil
		}
		if strings.Contains(strings.ToLower(c.Name), needle) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].ID, nil
	case 0:
		return "", fmt.Errorf("no course matches %q (see `canvas-cli courses`)", arg)
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = fmt.Sprintf("%s %s", m.CourseCode, m.Name)
		}
		return "", fmt.Errorf("%q is ambiguous: %s", arg, strings.Join(names, "; "))
	}
}
