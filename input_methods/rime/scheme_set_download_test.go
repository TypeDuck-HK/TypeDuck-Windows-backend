package rime

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gaboolic/moqi-ime/imecore"
)

func makeSchemeSetZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func withSchemeSetDownloadServer(t *testing.T, schemeName string, zipData []byte) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(zipData)
	archiveHash := hex.EncodeToString(sum[:])
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"packages":[{"name":%q,"url":%q,"sha256":%q}]}`, schemeName, server.URL+"/package.zip", archiveHash)
	})
	mux.HandleFunc("/package.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipData)
	})
	server = httptest.NewServer(mux)
	return server
}

func useSchemeSetDownloadServer(t *testing.T, server *httptest.Server) {
	t.Helper()
	t.Cleanup(resetSchemeSetDownloadStateForTest)
	schemeSetDownloadHTTPClient = server.Client()
	schemeSetDownloadPromptFunc = func(ctx context.Context) (schemeSetDownloadPackage, error) {
		return schemeSetDownloadPackage{
			Name: "AsyncSet",
			URL:  server.URL + "/package.zip",
		}, nil
	}
}

func TestInstallSchemeSetPackageDownloadsZipAndStripsTopLevel(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	zipData := makeSchemeSetZip(t, map[string]string{
		"repo-main/default.yaml":             "schema_list:\n",
		"repo-main/moqi_test.schema.yaml":    "schema:\n  schema_id: moqi_test\n",
		"repo-main/dicts/moqi.dict.yaml":     "# dict\n",
		"repo-main/recipes/full/recipe.yaml": "install_files: '*'\n",
	})
	server := withSchemeSetDownloadServer(t, "测试方案集", zipData)
	defer server.Close()
	useSchemeSetDownloadServer(t, server)

	name, err := installSchemeSetPackage(context.Background(), schemeSetDownloadPackage{
		Name: "测试方案集",
		URL:  server.URL + "/package.zip",
	})
	if err != nil {
		t.Fatalf("download and install scheme set: %v", err)
	}
	if name != "测试方案集" {
		t.Fatalf("unexpected scheme set name: %q", name)
	}

	root := filepath.Join(appData, APP, name)
	if _, err := os.Stat(filepath.Join(root, "default.yaml")); err != nil {
		t.Fatalf("expected default.yaml installed at scheme set root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "repo-main", "default.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected github top-level directory to be stripped, stat err=%v", err)
	}
	names := AvailableSchemeSetNames()
	found := false
	for _, candidate := range names {
		if candidate == name {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected installed scheme set to be listed, got %#v", names)
	}
}

func TestInstallSchemeSetPackageRejectsZipPathTraversal(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	zipData := makeSchemeSetZip(t, map[string]string{
		"../evil.txt": "bad",
	})
	server := withSchemeSetDownloadServer(t, "BadSet", zipData)
	defer server.Close()
	useSchemeSetDownloadServer(t, server)

	_, err := installSchemeSetPackage(context.Background(), schemeSetDownloadPackage{
		Name: "BadSet",
		URL:  server.URL + "/package.zip",
	})
	if err == nil {
		t.Fatal("expected path traversal archive to fail")
	}
	if !strings.Contains(err.Error(), "非法路径") {
		t.Fatalf("expected illegal path error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(appData, APP, "evil.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("expected traversal file not to be written, stat err=%v", statErr)
	}
}

func TestDownloadSchemeSetCommandInstallsSelectsAndRedeploys(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	zipData := makeSchemeSetZip(t, map[string]string{
		"repo-main/default.yaml":          "schema_list:\n",
		"repo-main/async.schema.yaml":     "schema:\n  schema_id: async\n",
		"repo-main/dicts/async.dict.yaml": "# dict\n",
	})
	server := withSchemeSetDownloadServer(t, "AsyncSet", zipData)
	defer server.Close()
	useSchemeSetDownloadServer(t, server)

	ime := newIsolatedTestIME(t)
	asyncResponses := make(chan *imecore.Response, 4)
	ime.SetAsyncResponseSender(func(resp *imecore.Response) {
		asyncResponses <- resp
	})

	resp := ime.onCommand(&imecore.Request{
		SeqNum: 200,
		ID:     imecore.FlexibleID{Int: ID_DOWNLOAD_SCHEME_SET, IsInt: true},
	}, imecore.NewResponse(200, true))
	if resp.ReturnValue != 1 {
		t.Fatalf("expected download command to start, got %d", resp.ReturnValue)
	}
	if resp.TrayNotification == nil || resp.TrayNotification.Message != "打开方案集下载窗口..." {
		t.Fatalf("unexpected start notification: %#v", resp.TrayNotification)
	}

	var success *imecore.Response
	timeout := time.After(2 * time.Second)
	for success == nil {
		select {
		case asyncResp := <-asyncResponses:
			if asyncResp.TrayNotification != nil && strings.Contains(asyncResp.TrayNotification.Message, "下载安装成功") {
				success = asyncResp
			}
		case <-timeout:
			t.Fatal("timed out waiting for async download completion")
		}
	}

	if current := CurrentSchemeSetName(); current != "AsyncSet" {
		t.Fatalf("expected downloaded scheme set selected, got %q", current)
	}
	backend := ime.backend.(*testBackend)
	if backend.redeployCalls != 1 {
		t.Fatalf("expected redeploy once, got %d", backend.redeployCalls)
	}
	if !strings.HasSuffix(filepath.Clean(backend.redeployUserDir), filepath.Join(APP, "AsyncSet")) {
		t.Fatalf("expected redeploy user dir to point at AsyncSet, got %q", backend.redeployUserDir)
	}
	if _, err := os.Stat(filepath.Join(appData, APP, "AsyncSet", "default.yaml")); err != nil {
		t.Fatalf("expected downloaded files installed: %v", err)
	}
}

func TestInstallSchemeSetPackageInfersNameFromURL(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	zipData := makeSchemeSetZip(t, map[string]string{
		"repo-main/default.yaml": "schema_list:\n",
	})
	server := withSchemeSetDownloadServer(t, "ignored", zipData)
	defer server.Close()
	useSchemeSetDownloadServer(t, server)

	name, err := installSchemeSetPackage(context.Background(), schemeSetDownloadPackage{
		URL: server.URL + "/package.zip",
	})
	if err != nil {
		t.Fatalf("install scheme set with inferred name: %v", err)
	}
	if name != "package" {
		t.Fatalf("expected inferred scheme set name, got %q", name)
	}
}

func TestNormalizeSchemeSetDownloadURLConvertsGitHubRepoToZip(t *testing.T) {
	got := normalizeSchemeSetDownloadURL("https://github.com/gaboolic/rime-frost")
	want := "https://github.com/gaboolic/rime-frost/archive/refs/heads/main.zip"
	if got != want {
		t.Fatalf("unexpected normalized URL: got %q want %q", got, want)
	}

	got = normalizeSchemeSetDownloadURL("https://github.com/gaboolic/rime-wubi-sentence/tree/master")
	want = "https://github.com/gaboolic/rime-wubi-sentence/archive/refs/heads/master.zip"
	if got != want {
		t.Fatalf("unexpected branch normalized URL: got %q want %q", got, want)
	}
}

func TestSchemeSetDownloadURLsFallbackFromMainToMaster(t *testing.T) {
	got := schemeSetDownloadURLs("https://github.com/gaboolic/rime-frost")
	want := []string{
		"https://github.com/gaboolic/rime-frost/archive/refs/heads/main.zip",
		"https://github.com/gaboolic/rime-frost/archive/refs/heads/master.zip",
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected fallback urls: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected fallback url %d: got %q want %q", i, got[i], want[i])
		}
	}
}
