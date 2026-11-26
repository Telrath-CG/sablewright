package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// buildInfo reads the info: block of build/config.yml. It is a handful of
// flat key: "value" lines, so it is read line by line rather than pulling in
// a YAML library for one test.
func buildInfo(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join("build", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info := map[string]string{}
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			in = strings.HasPrefix(line, "info:")
			continue
		}
		if !in {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value, _, _ = strings.Cut(value, " #")
		info[key] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return info
}

func readFile(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// plistString is the <string> that follows a <key> in an Info.plist.
func plistString(t *testing.T, plist, key string) string {
	t.Helper()
	m := regexp.MustCompile(`<key>` + regexp.QuoteMeta(key) + `</key>\s*<string>([^<]*)</string>`).FindStringSubmatch(plist)
	if m == nil {
		t.Fatalf("Info.plist has no string for %s", key)
	}
	return m[1]
}

// build/config.yml is where the version and the app's names are written
// down, but the platform files under build/ carry the names as fixed text and
// CI stamps the version into placeholders. Nothing else ties them together, so
// a name changed in one place and not the others would ship two different
// identities - and a placeholder CI can't find would ship version 0.0.0.
func TestBuildFilesAgreeWithConfig(t *testing.T) {
	info := buildInfo(t)

	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(info["version"]) {
		t.Errorf("info.version = %q, want a plain x.y.z version", info["version"])
	}
	if info["productIdentifier"] != appID {
		t.Errorf("productIdentifier = %q but main.go's appID is %q", info["productIdentifier"], appID)
	}

	plist := readFile(t, "build", "darwin", "Info.plist")
	for key, want := range map[string]string{
		"CFBundleIdentifier":         info["productIdentifier"],
		"CFBundleName":               info["productName"],
		"CFBundleExecutable":         "Sablewright",
		"CFBundleShortVersionString": "0.0.0",
		"CFBundleVersion":            "0.0.0",
	} {
		if got := plistString(t, plist, key); got != want {
			t.Errorf("Info.plist %s = %q, want %q", key, got, want)
		}
	}

	var winfo struct {
		Fixed map[string]string            `json:"fixed"`
		Info  map[string]map[string]string `json:"info"`
	}
	if err := json.Unmarshal([]byte(readFile(t, "build", "windows", "info.json")), &winfo); err != nil {
		t.Fatalf("build/windows/info.json: %v", err)
	}
	strs := winfo.Info["0409"]
	for key, want := range map[string]string{
		"CompanyName":    info["companyName"],
		"ProductName":    info["productName"],
		"FileVersion":    "0.0.0",
		"ProductVersion": "0.0.0",
	} {
		if got := strs[key]; got != want {
			t.Errorf("info.json %s = %q, want %q", key, got, want)
		}
	}
	if winfo.Fixed["file_version"] != "0.0.0" || winfo.Fixed["product_version"] != "0.0.0" {
		t.Errorf("info.json fixed versions = %v, want the 0.0.0 placeholders CI stamps", winfo.Fixed)
	}

	// The installer's names decide its uninstall entry, its install folder
	// and the file it installs, so they must stay what 1.x installed under
	// (the Wails v2 template's defaults) for 2.x to replace it in place rather
	// than install a second copy beside it.
	nsh := readFile(t, "build", "windows", "nsis", "wails_tools.nsh")
	for name, want := range map[string]string{
		"INFO_COMPANYNAME":    info["companyName"],
		"INFO_PRODUCTNAME":    info["productName"],
		"INFO_PROJECTNAME":    "sablewright",
		"PRODUCT_EXECUTABLE":  "${INFO_PROJECTNAME}.exe",
		"UNINST_KEY_NAME":     "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}",
		"WAILS_INSTALL_SCOPE": "machine",
	} {
		re := regexp.MustCompile(`!define ` + name + ` "([^"]*)"`)
		m := re.FindStringSubmatch(nsh)
		if m == nil || m[1] != want {
			t.Errorf("wails_tools.nsh %s = %v, want %q", name, m, want)
		}
	}
	nsi := readFile(t, "build", "windows", "nsis", "project.nsi")
	if !strings.Contains(nsi, `InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"`) {
		t.Error("project.nsi no longer installs to $PROGRAMFILES64\\<company>\\<product>, where 1.x is")
	}
}
