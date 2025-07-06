package main
import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
)
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	URL   string `json:"url"`
}
func (a *Author) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		a.Name = str
		return nil
	}
	type AuthorObj Author
	var obj AuthorObj
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	*a = Author(obj)
	return nil
}
func (a Author) String() string {
	if a.Name != "" {
		return a.Name
	}
	return ""
}
type Repository struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}
func (r *Repository) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		r.URL = str
		return nil
	}
	type RepoObj Repository
	var obj RepoObj
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	*r = Repository(obj)
	return nil
}
type Package struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	Main         string            `json:"main"`
	Scripts      map[string]string `json:"scripts"`
	Keywords     []string          `json:"keywords"`
	Author       Author            `json:"author"`
	License      string            `json:"license"`
	Bugs         struct {
		URL string `json:"url"`
	} `json:"bugs"`
	Homepage     string                 `json:"homepage"`
	Repository   Repository             `json:"repository"`
	Dependencies map[string]string      `json:"dependencies"`
	DevDeps      map[string]string      `json:"devDependencies"`
	Dist         struct {
		Tarball string `json:"tarball"`
		Shasum  string `json:"shasum"`
	} `json:"dist"`
}
type RegistryResponse struct {
	ID       string             `json:"_id"`
	Name     string             `json:"name"`
	Versions map[string]Package `json:"versions"`
	DistTags map[string]string  `json:"dist-tags"`
	Time     map[string]string  `json:"time"`
}
type PackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Description     string            `json:"description"`
	Main            string            `json:"main"`
	Scripts         map[string]string `json:"scripts"`
	Keywords        []string          `json:"keywords"`
	Author          string            `json:"author"`
	License         string            `json:"license"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}
type InstallTask struct {
	Name    string
	Version string
	Dir     string
	IsRoot  bool
}
type InstallResult struct {
	Task   InstallTask
	Error  error
	Size   int64
	Duration time.Duration
}
type UI struct {
	green   *color.Color
	red     *color.Color
	yellow  *color.Color
	blue    *color.Color
	cyan    *color.Color
	magenta *color.Color
	bold    *color.Color
}
func NewUI() *UI {
	return &UI{
		green:   color.New(color.FgGreen),
		red:     color.New(color.FgRed),
		yellow:  color.New(color.FgYellow),
		blue:    color.New(color.FgBlue),
		cyan:    color.New(color.FgCyan),
		magenta: color.New(color.FgMagenta),
		bold:    color.New(color.Bold),
	}
}
func (ui *UI) Success(msg string) {
	ui.green.Printf("%s\n", msg)
}
func (ui *UI) Error(msg string) {
	ui.red.Printf("%s\n", msg)
}
func (ui *UI) Warning(msg string) {
	ui.yellow.Printf("%s\n", msg)
}
func (ui *UI) Info(msg string) {
	ui.blue.Printf("%s\n", msg)
}
func (ui *UI) Spinner(msg string) {
	ui.cyan.Printf("⠋ %s", msg)
}
func (ui *UI) Header(msg string) {
	ui.bold.Printf("\n %s\n", msg)
	ui.cyan.Println(strings.Repeat("─", len(msg)+3))
}
const (
	NPM_REGISTRY_URL = "https://registry.npmjs.org"
	NODE_MODULES_DIR = "node_modules"
	MAX_CONCURRENT  = 10
)
var (
	ui = NewUI()
	httpClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
)
func main() {
	ui.Header("gopm - faster npm")
	if len(os.Args) < 2 {
		printUsage()
		return
	}
	command := os.Args[1]
	switch command {
	case "install", "i":
		if len(os.Args) < 3 {
			installFromPackageJSON()
		} else {
			packageName := os.Args[2]
			version := "latest"
			if len(os.Args) > 3 {
				version = os.Args[3]
			}
			installPackage(packageName, version)
		}
	case "uninstall", "rm":
		if len(os.Args) < 3 {
			ui.Error("usage: gopm uninstall <package>")
			return
		}
		uninstallPackage(os.Args[2])
	case "init":
		initPackage()
	case "info":
		if len(os.Args) < 3 {
			ui.Error("usage: gopm info <package-name>")
			return
		}
		showPackageInfo(os.Args[2])
	case "search":
		if len(os.Args) < 3 {
			ui.Error("usage: gopm search <query>")
			return
		}
		searchPackages(os.Args[2])
	case "list", "ls":
		listPackages()
	case "version":
		ui.Info("gopm version 1.0.0 haha 69")
	default:
		printUsage()
	}
}
func printUsage() {
	ui.Header("usage")
	fmt.Println("  gopm install [package] [version]   install package(s)")
	fmt.Println("  gopm uninstall [package] [version]   uninstall package(s)")
	fmt.Println("  gopm init                          initialize package.json")
	fmt.Println("  gopm info <package>                show package info")
	fmt.Println("  gopm search <query>                search packages")
	fmt.Println("  gopm list                          list installed packages")
	fmt.Println("  gopm version                       show version")
}
func installFromPackageJSON() {
	startTime := time.Now()
	packageJSON, err := readPackageJSON()
	if err != nil {
		ui.Error(fmt.Sprintf("error reading package.json: %v", err))
		return
	}
	if len(packageJSON.Dependencies) == 0 {
		ui.Warning("no dependencies found in package.json")
		return
	}
	ui.Header(fmt.Sprintf("installing %d dependencies", len(packageJSON.Dependencies)))
	tasks := make([]InstallTask, 0, len(packageJSON.Dependencies))
	for name, version := range packageJSON.Dependencies {
		tasks = append(tasks, InstallTask{
			Name:    name,
			Version: version,
			Dir:     NODE_MODULES_DIR,
			IsRoot:  true,
		})
	}
	results := installPackagesConcurrently(tasks)
	displayInstallResults(results, startTime)
}
func installPackage(name, version string) {
	startTime := time.Now()
	ui.Header(fmt.Sprintf("installing %s@%s", name, version))
	tasks := []InstallTask{{
		Name:    name,
		Version: version,
		Dir:     NODE_MODULES_DIR,
		IsRoot:  true,
	}}
	results := installPackagesConcurrently(tasks)
	displayInstallResults(results, startTime)
}
func uninstallPackage(name string) {
	ui.Header(fmt.Sprintf("uninstalling %s", name))
	packageDir := filepath.Join(NODE_MODULES_DIR, name)
	if _, err := os.Stat(packageDir); os.IsNotExist(err) {
		ui.Warning(fmt.Sprintf("%s is not installed", name))
		return
	}
	if err := os.RemoveAll(packageDir); err != nil {
		ui.Error(fmt.Sprintf("failed to remove %s: %v", name, err))
		return
	}
	packageJSON, err := readPackageJSON()
	if err != nil {
		ui.Error(fmt.Sprintf("error reading package.json: %v", err))
		return
	}
	changed := false
	if _, ok := packageJSON.Dependencies[name]; ok {
		delete(packageJSON.Dependencies, name)
		changed = true
	}
	if _, ok := packageJSON.DevDependencies[name]; ok {
		delete(packageJSON.DevDependencies, name)
		changed = true
	}
	if changed {
		file, err := os.Create("package.json")
		if err != nil {
			ui.Error(fmt.Sprintf("error updating package.json: %v", err))
			return
		}
		defer file.Close()
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(packageJSON); err != nil {
			ui.Error(fmt.Sprintf("error writing package.json: %v", err))
			return
		}
	}
	ui.Success(fmt.Sprintf("%s uninstalled", name))
}
func installPackagesConcurrently(tasks []InstallTask) []InstallResult {
	taskChan := make(chan InstallTask, len(tasks))
	resultChan := make(chan InstallResult, len(tasks))
	var wg sync.WaitGroup
	workerCount := min(MAX_CONCURRENT, len(tasks))
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskChan {
				result := processInstallTask(task)
				resultChan <- result
			}
		}()
	}
	for _, task := range tasks {
		taskChan <- task
	}
	close(taskChan)
	go func() {
		wg.Wait()
		close(resultChan)
	}()
	results := make([]InstallResult, 0, len(tasks))
	for result := range resultChan {
		results = append(results, result)
	}
	return results
}
func processInstallTask(task InstallTask) InstallResult {
	startTime := time.Now()
	registryData, err := getPackageFromRegistry(task.Name)
	if err != nil {
		return InstallResult{
			Task:     task,
			Error:    err,
			Duration: time.Since(startTime),
		}
	}
	version := task.Version
	if version == "latest" {
		version = registryData.DistTags["latest"]
	}
	packageData, exists := registryData.Versions[version]
	if !exists {
		for v := range registryData.Versions {
			if strings.HasPrefix(v, strings.TrimPrefix(version, "^")) ||
				strings.HasPrefix(v, strings.TrimPrefix(version, "~")) {
				version = v
				packageData = registryData.Versions[version]
				exists = true
				break
			}
		}
		if !exists {
			return InstallResult{
				Task:     task,
				Error:    fmt.Errorf("version %s not found", version),
				Duration: time.Since(startTime),
			}
		}
	}
	packageDir := filepath.Join(task.Dir, task.Name)
	if err := os.MkdirAll(task.Dir, 0755); err != nil {
		return InstallResult{
			Task:     task,
			Error:    err,
			Duration: time.Since(startTime),
		}
	}
	if _, err := os.Stat(packageDir); err == nil {
		return InstallResult{
			Task:     task,
			Error:    nil,
			Duration: time.Since(startTime),
		}
	}
	size, err := downloadAndExtractPackageEnhanced(packageData.Dist.Tarball, packageDir, task.Name)
	if err != nil {
		return InstallResult{
			Task:     task,
			Error:    err,
			Duration: time.Since(startTime),
		}
	}
	return InstallResult{
		Task:     task,
		Error:    nil,
		Size:     size,
		Duration: time.Since(startTime),
	}
}
func downloadAndExtractPackageEnhanced(tarballURL, destDir, packageName string) (int64, error) {
	resp, err := httpClient.Get(tarballURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to download: %s", resp.Status)
	}
	bar := progressbar.NewOptions64(
		resp.ContentLength,
		progressbar.OptionSetDescription(fmt.Sprintf(" %s", packageName)),
		progressbar.OptionSetWidth(30),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerPadding: "░",
			BarStart:      "▐",
			BarEnd:        "▌",
		}),
	)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, err
	}
	reader := io.TeeReader(resp.Body, bar)
	err = extractTarGz(reader, destDir)
	bar.Finish()
	return resp.ContentLength, err
}
func displayInstallResults(results []InstallResult, startTime time.Time) {
	successful := 0
	failed := 0
	totalSize := int64(0)
	for _, result := range results {
		if result.Error != nil {
			ui.Error(fmt.Sprintf("%s@%s: %v", result.Task.Name, result.Task.Version, result.Error))
			failed++
		} else {
			ui.Success(fmt.Sprintf("%s@%s installed in %v", result.Task.Name, result.Task.Version, result.Duration))
			successful++
			totalSize += result.Size
		}
	}
	totalTime := time.Since(startTime)
	ui.Header("installation summary")
	ui.Info(fmt.Sprintf("✓ %d successful, ✗ %d failed", successful, failed))
	ui.Info(fmt.Sprintf(" total size: %s", formatBytes(totalSize)))
	ui.Info(fmt.Sprintf(" total time: %v", totalTime))
	ui.Info(fmt.Sprintf(" average speed: %s/s", formatBytes(int64(float64(totalSize)/totalTime.Seconds()))))
}
func getPackageFromRegistry(name string) (*RegistryResponse, error) {
	url := fmt.Sprintf("%s/%s", NPM_REGISTRY_URL, name)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("package not found: %s", name)
	}
	var registryData RegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&registryData); err != nil {
		return nil, err
	}
	return &registryData, nil
}
func extractTarGz(src io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(header.Name, "package/")
		if name == "" {
			continue
		}
		target := filepath.Join(destDir, name)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", target)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}
func initPackage() {
	ui.Header("initializing package.json")
	packageJSON := PackageJSON{
		Name:            "a-package",
		Version:         "1.0.0",
		Description:     "swag",
		Main:            "index.js",
		Scripts:         map[string]string{
			"test":  "echo \"error: no test specified\" && exit 1",
			"start": "node index.js",
		},
		Keywords:        []string{"awesome", "swag"},
		Author:          "Your Name",
		License:         "MIT",
		Dependencies:    map[string]string{},
		DevDependencies: map[string]string{},
	}
	file, err := os.Create("package.json")
	if err != nil {
		ui.Error(fmt.Sprintf("error creating package.json: %v", err))
		return
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(packageJSON); err != nil {
		ui.Error(fmt.Sprintf("error writing package.json: %v", err))
		return
	}
	ui.Success("created package.json")
}
func showPackageInfo(name string) {
	ui.Header(fmt.Sprintf("package information: %s", name))
	registryData, err := getPackageFromRegistry(name)
	if err != nil {
		ui.Error(fmt.Sprintf("error fetching package info: %v", err))
		return
	}
	latest := registryData.DistTags["latest"]
	latestPackage := registryData.Versions[latest]
	ui.Info(fmt.Sprintf(" name: %s", registryData.Name))
	ui.Info(fmt.Sprintf(" version: %s", latest))
	ui.Info(fmt.Sprintf(" description: %s", latestPackage.Description))
	ui.Info(fmt.Sprintf(" author: %s", latestPackage.Author.String()))
	ui.Info(fmt.Sprintf(" license: %s", latestPackage.License))
	ui.Info(fmt.Sprintf(" homepage: %s", latestPackage.Homepage))
	if len(latestPackage.Keywords) > 0 {
		ui.Info(fmt.Sprintf("  keywords: %s", strings.Join(latestPackage.Keywords, ", ")))
	}
	if len(latestPackage.Dependencies) > 0 {
		ui.Info(fmt.Sprintf(" dependencies (%d):", len(latestPackage.Dependencies)))
		deps := make([]string, 0, len(latestPackage.Dependencies))
		for dep := range latestPackage.Dependencies {
			deps = append(deps, dep)
		}
		sort.Strings(deps)
		for _, dep := range deps {
			fmt.Printf("  • %s: %s\n", dep, latestPackage.Dependencies[dep])
		}
	}
}
func searchPackages(query string) {
	ui.Header(fmt.Sprintf("searching for: %s", query))
	url := fmt.Sprintf("https://registry.npmjs.org/-/v1/search?text=%s&size=20", query)
	resp, err := httpClient.Get(url)
	if err != nil {
		ui.Error(fmt.Sprintf("error searching packages: %v", err))
		return
	}
	defer resp.Body.Close()
	var searchResult struct {
		Objects []struct {
			Package struct {
				Name        string   `json:"name"`
				Version     string   `json:"version"`
				Description string   `json:"description"`
				Keywords    []string `json:"keywords"`
				Author      struct {
					Name string `json:"name"`
				} `json:"author"`
			} `json:"package"`
			Score struct {
				Final float64 `json:"final"`
			} `json:"score"`
		} `json:"objects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		ui.Error(fmt.Sprintf("error parsing search results: %v", err))
		return
	}
	ui.Info(fmt.Sprintf("found %d packages", len(searchResult.Objects)))
	for i, obj := range searchResult.Objects {
		pkg := obj.Package
		fmt.Printf("\n%d. %s", i+1, ui.bold.Sprintf("%s@%s", pkg.Name, pkg.Version))
		fmt.Printf("   score: %.2f\n", obj.Score.Final)
		if pkg.Description != "" {
			fmt.Printf("   %s\n", pkg.Description)
		}
		if pkg.Author.Name != "" {
			fmt.Printf("   %s\n", pkg.Author.Name)
		}
		if len(pkg.Keywords) > 0 {
			fmt.Printf("   %s\n", strings.Join(pkg.Keywords, ", "))
		}
	}
}
func listPackages() {
	ui.Header("installed Packages")
	nodeModulesDir := NODE_MODULES_DIR
	if _, err := os.Stat(nodeModulesDir); os.IsNotExist(err) {
		ui.Warning("no packages installed (node_modules directory not found)")
		return
	}
	entries, err := os.ReadDir(nodeModulesDir)
	if err != nil {
		ui.Error(fmt.Sprintf("error reading node_modules: %v", err))
		return
	}
	packages := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			packageJSONPath := filepath.Join(nodeModulesDir, entry.Name(), "package.json")
			if packageData, err := readPackageJSONFromPath(packageJSONPath); err == nil {
				packages = append(packages, fmt.Sprintf("%s@%s", packageData.Name, packageData.Version))
			} else {
				packages = append(packages, entry.Name())
			}
		}
	}
	sort.Strings(packages)
	ui.Info(fmt.Sprintf("found %d packages:", len(packages)))
	for i, pkg := range packages {
		fmt.Printf("  %d. %s\n", i+1, pkg)
	}
}
func readPackageJSON() (*PackageJSON, error) {
	return readPackageJSONFromPath("package.json")
}
func readPackageJSONFromPath(path string) (*PackageJSON, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var packageJSON PackageJSON
	if err := json.NewDecoder(file).Decode(&packageJSON); err != nil {
		return nil, err
	}
	return &packageJSON, nil
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
