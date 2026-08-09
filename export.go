package main

// Exporting one mini.
//
// Backup exists already and is a different thing: it is for getting your
// collection back, and its zip is meaningless to anyone else. This produces
// the thing you show someone - the photos, the paints, and the log of how it
// came together - in a file that outlives the app.
//
// Two formats, chosen by the extension in the save dialog. HTML is one
// self-contained file with the photos embedded in it, which opens in any
// browser and prints to PDF; Markdown is text for a forum or a club post,
// with the photos copied into a folder beside it.

import (
	"encoding/base64"
	"fmt"
	"html"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// exportPhotoMax is the long edge of the photos embedded in an export. Bigger
// than a thumbnail, because this one is meant to be looked at and printed;
// smaller than the original, because a page carrying eight 12-megapixel
// photos as base64 is a file nobody can send anywhere.
const exportPhotoMax = 1200

// ExportMini writes one mini out as a shareable file and returns the path.
// An empty path with no error means the save dialog was cancelled.
func (a *App) ExportMini(id int) (string, error) {
	m, ok := a.store.ModelByID(id)
	if !ok {
		return "", fmt.Errorf("that mini no longer exists")
	}
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Export " + m.Name,
		DefaultFilename: safeFilename(m.Name) + ".html",
		Filters: []wruntime.FileFilter{
			{DisplayName: "Web page, photos included (*.html)", Pattern: "*.html"},
			{DisplayName: "Markdown, photos alongside (*.md)", Pattern: "*.md"},
		},
	})
	if err != nil || path == "" {
		return "", err
	}

	paints := a.paintNames(m.PaintIDs)
	if strings.EqualFold(filepath.Ext(path), ".md") {
		return path, a.exportMarkdown(m, paints, path)
	}
	// Anything else gets the web page: a picker that returns no extension,
	// or a typed name that lost one, should not fail the export outright.
	if filepath.Ext(path) == "" {
		path += ".html"
	}
	return path, a.exportHTML(m, paints, path)
}

// paintNames resolves paint ids to name, brand and colour for the export.
func (a *App) paintNames(ids []int) []Paint {
	byID := map[int]Paint{}
	for _, p := range a.store.AllPaints() {
		byID[p.ID] = p
	}
	out := []Paint{}
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			out = append(out, p)
		}
	}
	return out
}

// safeFilename keeps a mini's name usable as one on every platform.
func safeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "mini"
	}
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimRight(b.String(), " .")
}

// exportPhoto re-encodes a photo down to a sensible size for sharing and
// returns it as a data URI. A photo that will not decode is skipped rather
// than breaking the export.
func (a *App) exportPhoto(file string) string {
	img, turn, err := decodeImage(filepath.Join(a.store.PhotoDir(), file))
	if err != nil {
		return ""
	}
	var buf strings.Builder
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	shot := orient(downscale(img, exportPhotoMax), turn)
	if err := jpeg.Encode(enc, shot, &jpeg.Options{Quality: 88}); err != nil {
		return ""
	}
	enc.Close()
	return "data:image/jpeg;base64," + buf.String()
}

func (a *App) exportHTML(m Model, paints []Paint, path string) error {
	esc := html.EscapeString
	total, done := m.Minis()
	var b strings.Builder

	b.WriteString(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>`)
	b.WriteString(esc(m.Name))
	b.WriteString(`</title>
<style>
 :root { color-scheme: light dark; }
 body { font: 16px/1.55 system-ui, -apple-system, "Segoe UI", sans-serif;
        max-width: 820px; margin: 0 auto; padding: 32px 20px 64px; }
 h1 { margin: 0 0 4px; font-size: 30px; }
 .sub { color: #6b7280; margin-bottom: 18px; }
 .facts { display: flex; flex-wrap: wrap; gap: 8px 18px; padding: 12px 0;
          border-top: 1px solid #d7dce3; border-bottom: 1px solid #d7dce3;
          font-size: 14px; color: #4b5563; }
 h2 { font-size: 13px; letter-spacing: .06em; text-transform: uppercase;
      color: #6b7280; margin: 28px 0 10px; }
 .shots { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
          gap: 10px; }
 .shots figure { margin: 0; }
 .shots img { width: 100%; border-radius: 8px; display: block; }
 .shots figcaption { font-size: 12px; color: #6b7280; margin-top: 4px; }
 ul.paints { list-style: none; padding: 0; display: flex; flex-wrap: wrap; gap: 8px; }
 ul.paints li { display: flex; align-items: center; gap: 7px; font-size: 14px;
                border: 1px solid #d7dce3; border-radius: 999px; padding: 4px 12px; }
 .dot { width: 13px; height: 13px; border-radius: 4px; border: 1px solid #0002; }
 .entry { border-left: 2px solid #d7dce3; padding: 0 0 14px 14px; }
 .when { font-size: 13px; color: #6b7280; }
 footer { margin-top: 40px; font-size: 12px; color: #9aa0a6; }
 @media print { body { max-width: none; } .shots { grid-template-columns: 1fr 1fr; } }
</style></head><body>
`)
	fmt.Fprintf(&b, "<h1>%s</h1>\n", esc(m.Name))
	if sub := strings.Join(nonEmpty(m.GameSystem, m.Faction, m.Project), " · "); sub != "" {
		fmt.Fprintf(&b, "<div class=\"sub\">%s</div>\n", esc(sub))
	}

	facts := []string{"Status: " + m.Status}
	if total > 1 {
		facts = append(facts, fmt.Sprintf("%d minis, %d painted", total, done))
	}
	if m.Started != "" {
		facts = append(facts, "Started "+m.Started)
	}
	if m.Completed != "" && finished(m.Status) {
		facts = append(facts, "Finished "+m.Completed)
	}
	if mins := m.TotalMinutes(); mins > 0 {
		facts = append(facts, fmt.Sprintf("%s at the desk over %s",
			humanMinutes(mins), pluralise(len(m.Sessions), "session")))
	}
	b.WriteString("<div class=\"facts\">")
	for _, f := range facts {
		fmt.Fprintf(&b, "<span>%s</span>", esc(f))
	}
	b.WriteString("</div>\n")

	// Reference shots are left out here and in the Markdown export below. An
	// export is the finished article and the work that went into it, and the
	// maker's marketing photograph is neither - handing someone a page that
	// opens on a studio paint job would misrepresent whose work it is.
	var shots []string
	for _, p := range m.Photos {
		if reference(p.Kind) {
			continue
		}
		if uri := a.exportPhoto(p.File); uri != "" {
			shots = append(shots, fmt.Sprintf(
				"<figure><img src=\"%s\" alt=\"\"><figcaption>%s</figcaption></figure>",
				uri, esc(p.Kind)))
		}
	}
	if len(shots) > 0 {
		b.WriteString("<h2>Photos</h2>\n<div class=\"shots\">")
		b.WriteString(strings.Join(shots, "\n"))
		b.WriteString("</div>\n")
	}

	if len(paints) > 0 {
		b.WriteString("<h2>Paints used</h2>\n<ul class=\"paints\">")
		for _, p := range paints {
			fmt.Fprintf(&b, "<li><span class=\"dot\" style=\"background:%s\"></span>%s</li>",
				esc(p.Hex), esc(strings.TrimSpace(p.Name+" "+bracket(p.Brand))))
		}
		b.WriteString("</ul>\n")
	}

	if notes := strings.TrimSpace(m.Notes); notes != "" {
		b.WriteString("<h2>Notes</h2>\n")
		for _, line := range strings.Split(notes, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				fmt.Fprintf(&b, "<p>%s</p>\n", esc(line))
			}
		}
	}

	if len(m.Sessions) > 0 {
		b.WriteString("<h2>Painting log</h2>\n")
		for _, e := range m.Sessions {
			when := e.Date
			if e.Minutes > 0 {
				when += " · " + humanMinutes(e.Minutes)
			}
			fmt.Fprintf(&b, "<div class=\"entry\"><div class=\"when\">%s</div><div>%s</div></div>\n",
				esc(when), strings.ReplaceAll(esc(e.Notes), "\n", "<br>"))
		}
	}

	b.WriteString("<footer>Exported from Sablewright on ")
	b.WriteString(today())
	b.WriteString("</footer>\n</body></html>\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func (a *App) exportMarkdown(m Model, paints []Paint, path string) error {
	total, done := m.Minis()
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", m.Name)
	if sub := strings.Join(nonEmpty(m.GameSystem, m.Faction, m.Project), " · "); sub != "" {
		fmt.Fprintf(&b, "*%s*\n\n", sub)
	}
	fmt.Fprintf(&b, "- **Status:** %s\n", m.Status)
	if total > 1 {
		fmt.Fprintf(&b, "- **Batch:** %d minis, %d painted\n", total, done)
	}
	if m.Started != "" {
		fmt.Fprintf(&b, "- **Started:** %s\n", m.Started)
	}
	if m.Completed != "" && finished(m.Status) {
		fmt.Fprintf(&b, "- **Finished:** %s\n", m.Completed)
	}
	if mins := m.TotalMinutes(); mins > 0 {
		fmt.Fprintf(&b, "- **Time at the desk:** %s over %s\n",
			humanMinutes(mins), pluralise(len(m.Sessions), "session"))
	}

	// The photos are copied rather than embedded, since Markdown has nowhere
	// to put an image. The folder is named after the file so the two stay
	// recognisably a pair wherever they end up.
	//
	// Reference shots are filtered before the folder is made rather than
	// skipped inside the loop: a mini whose only image is the product shot
	// would otherwise get an empty folder and a Photos heading with nothing
	// under it.
	var shots []Photo
	for _, p := range m.Photos {
		if !reference(p.Kind) {
			shots = append(shots, p)
		}
	}
	if len(shots) > 0 {
		dir := strings.TrimSuffix(path, filepath.Ext(path)) + "-photos"
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		b.WriteString("\n## Photos\n\n")
		for _, p := range shots {
			src := filepath.Join(a.store.PhotoDir(), p.File)
			raw, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, p.File), raw, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(&b, "![%s](%s/%s)\n\n", p.Kind, filepath.Base(dir), p.File)
		}
	}

	if len(paints) > 0 {
		b.WriteString("\n## Paints used\n\n")
		for _, p := range paints {
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(p.Name+" "+bracket(p.Brand)))
		}
	}
	if notes := strings.TrimSpace(m.Notes); notes != "" {
		fmt.Fprintf(&b, "\n## Notes\n\n%s\n", notes)
	}
	if len(m.Sessions) > 0 {
		b.WriteString("\n## Painting log\n\n")
		for _, e := range m.Sessions {
			when := e.Date
			if e.Minutes > 0 {
				when += " · " + humanMinutes(e.Minutes)
			}
			fmt.Fprintf(&b, "**%s**\n\n%s\n\n", when, e.Notes)
		}
	}
	fmt.Fprintf(&b, "\n---\n\nExported from Sablewright on %s\n", today())
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// humanMinutes renders 95 as "1h 35m", matching what the app shows on screen.
func humanMinutes(mins int) string {
	h, m := mins/60, mins%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

func pluralise(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func bracket(s string) string {
	if s = strings.TrimSpace(s); s == "" {
		return ""
	}
	return "(" + s + ")"
}

func nonEmpty(vals ...string) []string {
	out := []string{}
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
