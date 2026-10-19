package cmd

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/spf13/cobra"

	"github.com/bubunyo/canvas-cli/internal/canvas"
)

var (
	pullOut     string
	pullNoFiles bool
)

var pullCmd = &cobra.Command{
	Use:   "pull <course> [module...]",
	Short: "Save module content (pages, assignments, files) as markdown on disk",
	Long: `Downloads the content of a course's modules into
<out>/<COURSE CODE>/<NN - module name>/, one markdown file per page, assignment,
quiz or discussion prompt, plus attached files. Images and file links inside
pages are downloaded to files/ and rewritten to point at the local copies.

Modules are matched by id or by a case-insensitive fragment of their name
("week 3"); with no module arguments every module is pulled. Re-running skips
files that are already downloaded.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cl, err := client()
		if err != nil {
			return err
		}
		courseID, err := resolveCourse(ctx, cl, args[0])
		if err != nil {
			return err
		}
		var course canvas.Course
		if err := cl.Get(ctx, "/api/v1/courses/"+string(courseID), nil, &course); err != nil {
			return err
		}
		mods, err := courseModules(ctx, cl, courseID)
		if err != nil {
			return err
		}
		selected, err := selectModules(mods, args[1:])
		if err != nil {
			return err
		}

		root := filepath.Join(pullOut, sanitize(firstNonEmpty(course.CourseCode, string(course.ID))))
		for _, m := range selected {
			dir := filepath.Join(root, fmt.Sprintf("%02d - %s", m.Position, sanitize(m.Name)))
			fmt.Printf("%s → %s\n", m.Name, dir)
			p := &puller{cl: cl, courseID: courseID, dir: dir, files: map[string]string{}}
			if err := p.pullModule(ctx, m); err != nil {
				return err
			}
		}
		return nil
	},
}

func courseModules(ctx context.Context, cl *canvas.Client, id canvas.ID) ([]canvas.Module, error) {
	mods, err := canvas.GetAll[canvas.Module](ctx, cl, "/api/v1/courses/"+string(id)+"/modules", url.Values{"include[]": {"items"}})
	if err != nil {
		return nil, err
	}
	for i := range mods {
		// Large modules omit inline items; fetch them separately.
		if len(mods[i].Items) == 0 && mods[i].ItemsCount > 0 {
			items, err := canvas.GetAll[canvas.ModuleItem](ctx, cl, fmt.Sprintf("/api/v1/courses/%s/modules/%s/items", id, mods[i].ID), nil)
			if err != nil {
				return nil, err
			}
			mods[i].Items = items
		}
		// Canvas returns some names HTML-escaped ("Q&amp;A").
		mods[i].Name = html.UnescapeString(mods[i].Name)
		for j := range mods[i].Items {
			mods[i].Items[j].Title = html.UnescapeString(mods[i].Items[j].Title)
		}
	}
	return mods, nil
}

func selectModules(mods []canvas.Module, queries []string) ([]canvas.Module, error) {
	if len(queries) == 0 {
		return mods, nil
	}
	var out []canvas.Module
	for _, q := range queries {
		var matches []canvas.Module
		for _, m := range mods {
			if string(m.ID) == q {
				matches = []canvas.Module{m}
				break
			}
			if strings.Contains(strings.ToLower(m.Name), strings.ToLower(q)) {
				matches = append(matches, m)
			}
		}
		switch len(matches) {
		case 0:
			return nil, fmt.Errorf("no module matches %q (see `canvas modules <course>`)", q)
		case 1:
			out = append(out, matches[0])
		default:
			names := make([]string, len(matches))
			for i, m := range matches {
				names[i] = m.Name
			}
			return nil, fmt.Errorf("%q matches several modules: %s", q, strings.Join(names, "; "))
		}
	}
	return out, nil
}

type puller struct {
	cl       *canvas.Client
	courseID canvas.ID
	dir      string
	files    map[string]string // file id → path relative to dir
}

func (p *puller) pullModule(ctx context.Context, m canvas.Module) error {
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return err
	}
	var index strings.Builder
	fmt.Fprintf(&index, "# %s\n\n", m.Name)

	for n, it := range m.Items {
		indent := strings.Repeat("  ", it.Indent)
		base := fmt.Sprintf("%02d - %s", n+1, sanitize(it.Title))
		rel, err := p.pullItem(ctx, it, base)
		switch {
		case errors.Is(err, canvas.ErrUnauthorized):
			return err
		case err != nil:
			fmt.Printf("  ✗ %s%s (%s): %v\n", indent, it.Title, it.Type, err)
			fmt.Fprintf(&index, "%s- %s _(%s — not downloaded: %v)_\n", indent, it.Title, it.Type, err)
		case it.Type == "SubHeader":
			fmt.Fprintf(&index, "\n%s**%s**\n\n", indent, it.Title)
		case rel == "":
			link := firstNonEmpty(it.ExternalURL, it.HTMLURL)
			fmt.Printf("  · %s%s (%s, link only)\n", indent, it.Title, it.Type)
			fmt.Fprintf(&index, "%s- [%s](%s) _(%s)_\n", indent, it.Title, link, it.Type)
		default:
			fmt.Printf("  ✓ %s%s\n", indent, rel)
			fmt.Fprintf(&index, "%s- [%s](%s)\n", indent, it.Title, (&url.URL{Path: rel}).String())
		}
	}
	return os.WriteFile(filepath.Join(p.dir, "README.md"), []byte(index.String()), 0o644)
}

// pullItem saves one module item and returns its path relative to p.dir,
// or "" for items that are only linked (external URLs, LTI tools, headers).
func (p *puller) pullItem(ctx context.Context, it canvas.ModuleItem, base string) (string, error) {
	c := "/api/v1/courses/" + string(p.courseID)
	switch it.Type {
	case "Page":
		var pg canvas.Page
		if err := p.cl.Get(ctx, c+"/pages/"+it.PageURL, nil, &pg); err != nil {
			return "", err
		}
		return p.writeMarkdown(ctx, base, pg.Title, pg.Body, meta{"Source": pg.HTMLURL, "Updated": fmtTime(pg.UpdatedAt)})
	case "Assignment":
		var a canvas.Assignment
		if err := p.cl.Get(ctx, c+"/assignments/"+string(it.ContentID), nil, &a); err != nil {
			return "", err
		}
		return p.writeMarkdown(ctx, base, a.Name, a.Description, meta{"Source": a.HTMLURL, "Due": fmtTime(a.DueAt), "Points": fmtPoints(a.PointsPossible)})
	case "Quiz":
		var q canvas.Quiz
		if err := p.cl.Get(ctx, c+"/quizzes/"+string(it.ContentID), nil, &q); err != nil {
			return "", err
		}
		return p.writeMarkdown(ctx, base, q.Title, q.Description, meta{"Source": q.HTMLURL, "Due": fmtTime(q.DueAt), "Points": fmtPoints(q.PointsPossible)})
	case "Discussion":
		// Only the prompt; other students' replies are deliberately not saved.
		var d canvas.DiscussionTopic
		if err := p.cl.Get(ctx, c+"/discussion_topics/"+string(it.ContentID), nil, &d); err != nil {
			return "", err
		}
		return p.writeMarkdown(ctx, base, d.Title, d.Message, meta{"Source": d.HTMLURL})
	case "File":
		if pullNoFiles {
			return "", nil
		}
		f, err := p.fileInfo(ctx, it.ContentID)
		if err != nil {
			return "", err
		}
		name := fmt.Sprintf("%s%s", base, filepath.Ext(firstNonEmpty(f.DisplayName, f.Filename)))
		return name, p.download(ctx, f, name)
	default: // SubHeader, ExternalUrl, ExternalTool
		return "", nil
	}
}

type meta map[string]string

func (p *puller) writeMarkdown(ctx context.Context, base, title, body string, m meta) (string, error) {
	body = p.localizeFiles(ctx, body)
	md, err := htmltomarkdown.ConvertString(body)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	for _, k := range []string{"Source", "Due", "Points", "Updated"} {
		if v := m[k]; v != "" && v != "-" {
			fmt.Fprintf(&b, "> **%s:** %s  \n", k, v)
		}
	}
	fmt.Fprintf(&b, "\n%s\n", strings.TrimSpace(md))
	name := base + ".md"
	return name, os.WriteFile(filepath.Join(p.dir, name), []byte(b.String()), 0o644)
}

var (
	attrURL  = regexp.MustCompile(`(src|href)="([^"]+)"`)
	fileLink = regexp.MustCompile(`/files/(\d+)`)
)

// localizeFiles downloads Canvas files referenced by src/href attributes
// (embedded images, linked documents) into files/ and rewrites the links to
// the local copies. Other root-relative Canvas links are made absolute.
func (p *puller) localizeFiles(ctx context.Context, body string) string {
	base, _ := url.Parse(p.cl.BaseURL)
	return attrURL.ReplaceAllStringFunc(body, func(attr string) string {
		m := attrURL.FindStringSubmatch(attr)
		u, err := url.Parse(strings.ReplaceAll(m[2], "&amp;", "&"))
		if err != nil || (u.Host != "" && u.Host != base.Host) {
			return attr
		}
		absolute := attr
		if u.Host == "" && strings.HasPrefix(u.Path, "/") {
			absolute = fmt.Sprintf(`%s="%s"`, m[1], base.ResolveReference(u))
		}
		fm := fileLink.FindStringSubmatch(u.Path)
		if fm == nil || pullNoFiles {
			return absolute
		}
		rel, err := p.localFile(ctx, canvas.ID(fm[1]))
		if err != nil {
			fmt.Fprintf(os.Stderr, "    warning: file %s: %v\n", fm[1], err)
			return absolute
		}
		return fmt.Sprintf(`%s="%s"`, m[1], (&url.URL{Path: rel}).String())
	})
}

func (p *puller) localFile(ctx context.Context, id canvas.ID) (string, error) {
	if rel, ok := p.files[string(id)]; ok {
		return rel, nil
	}
	f, err := p.fileInfo(ctx, id)
	if err != nil {
		return "", err
	}
	rel := filepath.Join("files", fmt.Sprintf("%s-%s", id, sanitize(firstNonEmpty(f.DisplayName, f.Filename))))
	if err := p.download(ctx, f, rel); err != nil {
		return "", err
	}
	p.files[string(id)] = rel
	return rel, nil
}

func (p *puller) fileInfo(ctx context.Context, id canvas.ID) (*canvas.File, error) {
	var f canvas.File
	if err := p.cl.Get(ctx, "/api/v1/courses/"+string(p.courseID)+"/files/"+string(id), nil, &f); err != nil {
		return nil, err
	}
	if f.URL == "" {
		return nil, errors.New("no download URL (file locked or hidden)")
	}
	return &f, nil
}

// download writes f to rel (relative to p.dir), skipping it if a file of the
// same size is already there.
func (p *puller) download(ctx context.Context, f *canvas.File, rel string) error {
	dst := filepath.Join(p.dir, rel)
	if st, err := os.Stat(dst); err == nil && st.Size() == f.Size {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = p.cl.Download(ctx, f.URL, out)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

var unsafeChars = regexp.MustCompile(`[/\\:*?"<>|\x00-\x1f]+`)

func sanitize(s string) string {
	s = strings.Join(strings.Fields(unsafeChars.ReplaceAllString(s, "-")), " ")
	s = strings.Trim(s, ". -")
	if r := []rune(s); len(r) > 100 {
		s = strings.TrimSpace(string(r[:100]))
	}
	if s == "" {
		s = "untitled"
	}
	return s
}

func fmtPoints(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%g", *p)
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func init() {
	pullCmd.Flags().StringVarP(&pullOut, "out", "o", ".", "directory to write into")
	pullCmd.Flags().BoolVar(&pullNoFiles, "no-files", false, "skip downloading files and embedded images")
	rootCmd.AddCommand(pullCmd)
}
