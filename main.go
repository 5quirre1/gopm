
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
	"runtime"
	"regexp"
    "strconv"
)
type Author struct {
    Name  string `json:"name"`
    Email string `json:"email"`
    URL   string `json:"url"`
}
type License struct {
    Type string `json:"type"`
    URL  string `json:"url"`
}
func (l *License) UnmarshalJSON(data []byte) error {
    var str string
    if err := json.Unmarshal(data, &str); err == nil {
        l.Type = str
        return nil
    }
    type LicenseObj License
    var obj LicenseObj
    if err := json.Unmarshal(data, &obj); err != nil {
        return err
    }
    *l = License(obj)
    return nil
}
func (a *Author) UnmarshalJSON(data []byte) error {
    var str string
    if err := json.Unmarshal(data, &str); err == nil {
        a.Name = str
        return nil
    }
    type AuthorAlias Author
    var tmp AuthorAlias
    if err := json.Unmarshal(data, &tmp); err != nil {
        return err
    }
    *a = Author(tmp)
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
	License      interface{}       `json:"license"`
	Bugs         struct {
		URL string `json:"url"`
	} `json:"bugs"`
	Homepage     interface{} `json:"homepage"`
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
    Name            string                 `json:"name"`
    Version         string                 `json:"version"`
    Description     string                 `json:"description"`
    Main            string                 `json:"main"`
    Bin             interface{}            `json:"bin"`
    Scripts         map[string]string      `json:"scripts"`
    Keywords        []string               `json:"keywords"`
    Author          string                 `json:"author"`
    License         string                 `json:"license"`
    Dependencies    map[string]string      `json:"dependencies"`
    DevDependencies map[string]string      `json:"devDependencies"`
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
func getGlobalInstallDir() (string, error) {
	if customRoot := os.Getenv("GOPM_ROOT"); customRoot != "" {
		return filepath.Join(customRoot, "lib", "node_modules"), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(homeDir, "AppData", "Roaming", "npm", "node_modules"), nil
	default:
		return filepath.Join(homeDir, ".npm-global", "lib", "node_modules"), nil
	}
}
func showLocalRoot() {
    cwd, err := os.Getwd()
    if err != nil {
        ui.Error(fmt.Sprintf("failed to get current directory: %v", err))
        return
    }
    localRoot := filepath.Join(cwd, "node_modules")
    ui.Header("local node_modules directory")
    ui.Info(fmt.Sprintf("Path: %s", localRoot))
    if stat, err := os.Stat(localRoot); err == nil {
        ui.Info(fmt.Sprintf("\nDirectory exists, last modified: %s", stat.ModTime().Format(time.RFC1123)))
    } else {
        ui.Warning("\ndirectory does not exist yet")
    }
}
func showGlobalRoot() {
    globalDir, err := getGlobalInstallDir()
    if err != nil {
        ui.Error(fmt.Sprintf("failed to determine global directory: %v", err))
        return
    }
    ui.Header("Global node_modules directory")
    ui.Info(fmt.Sprintf("Path: %s", globalDir))
    if stat, err := os.Stat(globalDir); err == nil {
        ui.Info(fmt.Sprintf("\nDirectory exists, last modified: %s", stat.ModTime().Format(time.RFC1123)))
    } else {
        ui.Warning("\nDirectory does not exist yet")
    }
}
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
			return
		}
		if os.Args[2] == "-g" {
			if len(os.Args) < 4 {
				ui.Error("usage: gopm install -g <package> [version]")
				return
			}
			packageName := os.Args[3]
			version := "latest"
			if len(os.Args) > 4 {
				version = os.Args[4]
			}
			installPackageGlobal(packageName, version)
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
			ui.Error("usage: gopm uninstall <package> [-g]")
			return
		}
		if len(os.Args) > 3 && os.Args[3] == "-g" {
			uninstallPackageGlobal(os.Args[2])
		} else {
			uninstallPackage(os.Args[2])
		}
	case "update":
		if len(os.Args) == 2 {
			updateAllPackages()
		} else {
			updatePackage(os.Args[2])
		}
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
		if len(os.Args) > 2 && os.Args[2] == "-g" {
			listGlobalPackages()
		} else {
			listPackages()
		}
	case "version":
		ui.Info("gopm version 1.1.1")
	case "root":
	    if len(os.Args) > 2 && os.Args[2] == "-g" {
	        showGlobalRoot()
	    } else {
	        showLocalRoot()
	    }
	default:
		printUsage()
	}
}
func printUsage() {
	ui.Header("usage")
	fmt.Println("  gopm install [-g] [package] [version]   install package(s) (global if -g)")
	fmt.Println("  gopm root [-g]                     show node_modules directory path (global if -g)")
	fmt.Println("  gopm uninstall [package] [-g]      uninstall package(s) (global if -g)")
	fmt.Println("  gopm update [package]              update one or all packages to latest version (global if -g)")
	fmt.Println("  gopm init                          initialize package.json")
	fmt.Println("  gopm info <package>                show package info")
	fmt.Println("  gopm search <query>                search packages")
	fmt.Println("  gopm list [-g]                     list installed packages (global if -g)")
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
    var queue []InstallTask
    installed := make(map[string]bool)
    for name, version := range packageJSON.Dependencies {
        queue = append(queue, InstallTask{
            Name:    name,
            Version: version,
            Dir:     NODE_MODULES_DIR,
            IsRoot:  true,
        })
    }
    var results []InstallResult
    var mu sync.Mutex
    for len(queue) > 0 {
        currentBatch := queue
        queue = nil
        batchResults := installPackagesConcurrently(currentBatch)
        results = append(results, batchResults...)
        var wg sync.WaitGroup
        for _, result := range batchResults {
            if result.Error != nil {
                continue
            }
            wg.Add(1)
            go func(task InstallTask) {
                defer wg.Done()
                packageDir := filepath.Join(task.Dir, task.Name)
                packageJSONPath := filepath.Join(packageDir, "package.json")
                pkgJSON, err := readPackageJSONFromPath(packageJSONPath)
                if err != nil {
                    return
                }
                mu.Lock()
                defer mu.Unlock()
                for depName, depVersion := range pkgJSON.Dependencies {
                    if !installed[depName] {
                        installed[depName] = true
                        queue = append(queue, InstallTask{
                            Name:    depName,
                            Version: depVersion,
                            Dir:     filepath.Join(packageDir, "node_modules"),
                            IsRoot:  false,
                        })
                    }
                }
            }(result.Task)
        }
        wg.Wait()
    }
    if err := linkLocalBinaries(); err != nil {
        ui.Error(fmt.Sprintf("failed to link binaries: %v", err))
    }
    displayInstallResults(results, startTime)
    localBinPath := filepath.Join(NODE_MODULES_DIR, ".bin")
    ui.Info("\nto use locally installed binaries, add to your PATH:")
    ui.Info(fmt.Sprintf("  export PATH=$PATH:%s", localBinPath))
}
func installPackage(name, version string) {
    startTime := time.Now()
    ui.Header(fmt.Sprintf("installing %s@%s", name, version))
    if strings.HasPrefix(name, "@") {
        parts := strings.Split(name, "/")
        if len(parts) != 2 {
            ui.Error("invalid scoped package name")
            return
        }
    }
    tasks := []InstallTask{{
        Name:    name,
        Version: version,
        Dir:     NODE_MODULES_DIR,
        IsRoot:  true,
    }}
    results := installPackagesConcurrently(tasks)
    if len(results) == 0 || results[0].Error != nil {
        displayInstallResults(results, startTime)
        return
    }
    packageDir := filepath.Join(NODE_MODULES_DIR, name)
    if strings.HasPrefix(name, "@") {
        parts := strings.Split(name, "/")
        packageDir = filepath.Join(NODE_MODULES_DIR, parts[0], parts[1])
    }
    packageJSONPath := filepath.Join(packageDir, "package.json")
    pkgJSON, err := readPackageJSONFromPath(packageJSONPath)
    if err != nil {
        ui.Error(fmt.Sprintf("failed to read package.json for %s: %v", name, err))
        return
    }
    if len(pkgJSON.Dependencies) > 0 {
        ui.Info(fmt.Sprintf("installing %d dependencies for %s", len(pkgJSON.Dependencies), name))
        depTasks := make([]InstallTask, 0, len(pkgJSON.Dependencies))
        for depName, depVersion := range pkgJSON.Dependencies {
            depDir := filepath.Join(packageDir, "node_modules")
            if strings.HasPrefix(depName, "@") {
                parts := strings.Split(depName, "/")
                depDir = filepath.Join(depDir, parts[0])
            }
            depTasks = append(depTasks, InstallTask{
                Name:    depName,
                Version: depVersion,
                Dir:     depDir,
                IsRoot:  false,
            })
        }
        depResults := installPackagesConcurrently(depTasks)
        results = append(results, depResults...)
    }
    if err := linkLocalBinaries(); err != nil {
        ui.Error(fmt.Sprintf("failed to link binaries: %v", err))
    }
    if shouldUpdatePackageJSON(name) {
        if err := addToPackageJSON(name, version); err != nil {
            ui.Error(fmt.Sprintf("failed to update package.json: %v", err))
        } else {
            ui.Info("updated package.json")
        }
    }
    displayInstallResults(results, startTime)
    localBinPath := filepath.Join(NODE_MODULES_DIR, ".bin")
    ui.Info("\nto use locally installed binaries, add to your PATH:")
    ui.Info(fmt.Sprintf("  export PATH=$PATH:%s", localBinPath))
    ui.Info("or run directly with:")
    ui.Info(fmt.Sprintf("  ./node_modules/.bin/%s", name))
}
func linkLocalBinaries() error {
    binDir := filepath.Join(NODE_MODULES_DIR, ".bin")
    if err := os.MkdirAll(binDir, 0755); err != nil {
        return err
    }
    entries, err := os.ReadDir(NODE_MODULES_DIR)
    if err != nil {
        return err
    }
    for _, entry := range entries {
        if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
            packageDir := filepath.Join(NODE_MODULES_DIR, entry.Name())
            packageJSONPath := filepath.Join(packageDir, "package.json")
            pkgJSON, err := readPackageJSONFromPath(packageJSONPath)
            if err != nil {
                continue
            }
            if pkgJSON.Bin != nil {
                switch bin := pkgJSON.Bin.(type) {
                case string:
                    src := filepath.Join(packageDir, bin)
                    dest := filepath.Join(binDir, entry.Name())
                    if err := createBinLink(src, dest); err != nil {
                        return fmt.Errorf("failed to link %s: %v", entry.Name(), err)
                    }
                case map[string]interface{}:
                    for name, path := range bin {
                        if pathStr, ok := path.(string); ok {
                            src := filepath.Join(packageDir, pathStr)
                            dest := filepath.Join(binDir, name)
                            if err := createBinLink(src, dest); err != nil {
                                return fmt.Errorf("failed to link %s: %v", name, err)
                            }
                        }
                    }
                }
            }
        }
    }
    return nil
}
func createBinLink(src, dest string) error {
    if _, err := os.Stat(src); os.IsNotExist(err) {
        return fmt.Errorf("source binary does not exist: %s", src)
    }
    if _, err := os.Lstat(dest); err == nil {
        os.Remove(dest)
    }
    if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
        return err
    }
    relPath, err := filepath.Rel(filepath.Dir(dest), src)
    if err != nil {
        return err
    }
    return os.Symlink(relPath, dest)
}
func shouldUpdatePackageJSON(packageName string) bool {
    if _, err := os.Stat("package.json"); os.IsNotExist(err) {
        return false
    }
    for _, arg := range os.Args {
        if arg == packageName {
            return true
        }
    }
    return false
}
func addToPackageJSON(name, version string) error {
    pkgJSON, err := readPackageJSON()
    if err != nil {
        return err
    }
    if pkgJSON.Dependencies == nil {
        pkgJSON.Dependencies = make(map[string]string)
    }
    if _, exists := pkgJSON.Dependencies[name]; !exists {
        pkgJSON.Dependencies[name] = version
    }
    data, err := json.MarshalIndent(pkgJSON, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile("package.json", data, 0644)
}
func installPackageGlobal(name, version string) {
    startTime := time.Now()
    globalDir, err := getGlobalInstallDir()
    if err != nil {
        ui.Error(fmt.Sprintf("failed to determine global installation directory: %v", err))
        return
    }
    binDir := filepath.Dir(globalDir)
    binDir = filepath.Join(binDir, "bin")
    if err := os.MkdirAll(binDir, 0755); err != nil {
        ui.Error(fmt.Sprintf("failed to create bin directory: %v", err))
        return
    }
    if err := os.MkdirAll(globalDir, 0755); err != nil {
        ui.Error(fmt.Sprintf("failed to create global directory: %v", err))
        return
    }
    ui.Header(fmt.Sprintf("installing %s@%s globally", name, version))
    tasks := []InstallTask{{
        Name:    name,
        Version: version,
        Dir:     globalDir,
        IsRoot:  true,
    }}
    results := installPackagesConcurrently(tasks)
    if len(results) == 0 || results[0].Error != nil {
        displayInstallResults(results, startTime)
        return
    }
    packageDir := filepath.Join(globalDir, name)
    packageJSONPath := filepath.Join(packageDir, "package.json")
    pkgJSON, err := readPackageJSONFromPath(packageJSONPath)
    if err != nil {
        ui.Error(fmt.Sprintf("failed to read package.json for %s: %v", name, err))
        return
    }
    if len(pkgJSON.Dependencies) > 0 {
        ui.Info(fmt.Sprintf("installing %d dependencies for %s", len(pkgJSON.Dependencies), name))
        depTasks := make([]InstallTask, 0, len(pkgJSON.Dependencies))
        for depName, depVersion := range pkgJSON.Dependencies {
            depTasks = append(depTasks, InstallTask{
                Name:    depName,
                Version: depVersion,
                Dir:     filepath.Join(packageDir, "node_modules"),
                IsRoot:  false,
            })
        }
        depResults := installPackagesConcurrently(depTasks)
        results = append(results, depResults...)
    }
    if err := linkGlobalBinaries(packageDir, binDir); err != nil {
        ui.Error(fmt.Sprintf("failed to link binaries: %v", err))
    }
    displayInstallResults(results, startTime)
    ui.Info(fmt.Sprintf("package installed globally to: %s", globalDir))
    ui.Info(fmt.Sprintf("binaries linked to: %s", binDir))
    pathEnv := os.Getenv("PATH")
    if !strings.Contains(pathEnv, binDir) {
        ui.Warning("\nglobal bin directory not found in PATH. add this to your shell configuration:")
        ui.Info(fmt.Sprintf("  export PATH=$PATH:%s", binDir))
    }
}
func linkGlobalBinaries(packageDir, binDir string) error {
    packageJSONPath := filepath.Join(packageDir, "package.json")
    pkgJSON, err := readPackageJSONFromPath(packageJSONPath)
    if err != nil {
        return err
    }
    if pkgJSON.Bin == nil {
        return nil
    }
    switch bin := pkgJSON.Bin.(type) {
    case string:
        src := filepath.Join(packageDir, bin)
        dest := filepath.Join(binDir, filepath.Base(pkgJSON.Name))
        return createSymlink(src, dest)
    case map[string]interface{}:
        for name, path := range bin {
            if pathStr, ok := path.(string); ok {
                src := filepath.Join(packageDir, pathStr)
                dest := filepath.Join(binDir, name)
                if err := createSymlink(src, dest); err != nil {
                    return err
                }
            }
        }
    }
    return nil
}
func createSymlink(src, dest string) error {
    if _, err := os.Lstat(dest); err == nil {
        os.Remove(dest)
    }
    if runtime.GOOS == "windows" {
cmdContent := fmt.Sprintf("@ECHO OFF\r\n\"%%~dp0\\%s\" %%*\r\n", filepath.Base(src))
return os.WriteFile(dest+".cmd", []byte(cmdContent), 0644)
    } else {
        relPath, err := filepath.Rel(filepath.Dir(dest), src)
        if err != nil {
            return err
        }
        return os.Symlink(relPath, dest)
    }
}
func uninstallPackage(name string) {
	ui.Header(fmt.Sprintf("uninstalling %s", name))
	packageDir := filepath.Join(NODE_MODULES_DIR, name)
	if _, err := os.Stat(packageDir); os.IsNotExist(err) {
		ui.Warning(fmt.Sprintf("package '%s' is not installed", name))
		return
	}
	err := os.RemoveAll(packageDir)
	if err != nil {
		ui.Error(fmt.Sprintf("failed to uninstall %s: %v", name, err))
		return
	}
	ui.Success(fmt.Sprintf("uninstalled %s", name))
	pkgJsonPath := "package.json"
	file, err := os.ReadFile(pkgJsonPath)
	if err != nil {
		return
	}
	var packageJSON PackageJSON
	err = json.Unmarshal(file, &packageJSON)
	if err != nil {
		return
	}
	if _, ok := packageJSON.Dependencies[name]; ok {
		delete(packageJSON.Dependencies, name)
		updated, err := json.MarshalIndent(packageJSON, "", "  ")
		if err == nil {
			_ = os.WriteFile(pkgJsonPath, updated, 0644)
			ui.Info("updated package.json")
		}
	}
}
func uninstallPackageGlobal(name string) {
    ui.Header(fmt.Sprintf("uninstalling global package %s", name))
    globalDir, err := getGlobalInstallDir()
    if err != nil {
        ui.Error(fmt.Sprintf("failed to determine global directory: %v", err))
        return
    }
    packageDir := filepath.Join(globalDir, name)
    if _, err := os.Stat(packageDir); os.IsNotExist(err) {
        ui.Warning(fmt.Sprintf("global package '%s' is not installed", name))
        return
    }
    err = os.RemoveAll(packageDir)
    if err != nil {
        ui.Error(fmt.Sprintf("failed to uninstall %s: %v", name, err))
        return
    }
    ui.Success(fmt.Sprintf("uninstalled global package %s", name))
}
func updatePackage(name string) {
	ui.Header(fmt.Sprintf("updating package: %s", name))
	packageJSON, err := readPackageJSON()
	if err != nil {
		ui.Error(fmt.Sprintf("error reading package.json: %v", err))
		return
	}
	if _, ok := packageJSON.Dependencies[name]; !ok {
		ui.Warning(fmt.Sprintf("package '%s' is not in dependencies", name))
		return
	}
	tasks := []InstallTask{{
		Name:    name,
		Version: "latest",
		Dir:     NODE_MODULES_DIR,
		IsRoot:  true,
	}}
	results := installPackagesConcurrently(tasks)
	for _, result := range results {
		if result.Error == nil {
			packageJSON.Dependencies[name] = result.Task.Version
		}
	}
	data, err := json.MarshalIndent(packageJSON, "", "  ")
	if err == nil {
		_ = os.WriteFile("package.json", data, 0644)
		ui.Info("updated package.json")
	}
	displayInstallResults(results, time.Now())
}
func updateAllPackages() {
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
	ui.Header("updating all dependencies to latest versions")
	tasks := make([]InstallTask, 0, len(packageJSON.Dependencies))
	for name := range packageJSON.Dependencies {
		tasks = append(tasks, InstallTask{
			Name:    name,
			Version: "latest",
			Dir:     NODE_MODULES_DIR,
			IsRoot:  true,
		})
	}
	results := installPackagesConcurrently(tasks)
	for _, result := range results {
		if result.Error == nil {
			packageJSON.Dependencies[result.Task.Name] = result.Task.Version
		}
	}
	data, err := json.MarshalIndent(packageJSON, "", "  ")
	if err == nil {
		_ = os.WriteFile("package.json", data, 0644)
		ui.Info("updated package.json with latest versions")
	}
	displayInstallResults(results, startTime)
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
    actualPkgName := task.Name
    if strings.HasPrefix(task.Version, "npm:") {
        parts := strings.Split(task.Version, "@")
        if len(parts) >= 2 {
            actualPkgName = parts[0][4:]
            task.Version = parts[len(parts)-1]
        }
    }
    registryData, err := getPackageFromRegistry(actualPkgName)
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
    } else if _, err := strconv.Atoi(version); err == nil {
        var matchingVersions []string
        for v := range registryData.Versions {
            if strings.HasPrefix(v, version+".") {
                matchingVersions = append(matchingVersions, v)
            }
        }
        if len(matchingVersions) > 0 {
            sort.Slice(matchingVersions, func(i, j int) bool {
                return compareVersions(matchingVersions[i], matchingVersions[j]) > 0
            })
            version = matchingVersions[0]
        } else {
            return InstallResult{
                Task:     task,
                Error:    fmt.Errorf("no matching version found for %s (tried %s)", task.Version, strings.Join(getAllVersions(registryData.Versions), ", ")),
                Duration: time.Since(startTime),
            }
        }
    }
    var packageData Package
    var resolvedVersion string
    if pkg, exists := registryData.Versions[version]; exists {
        packageData = pkg
        resolvedVersion = version
    } else {
        versions := make([]string, 0, len(registryData.Versions))
        for v := range registryData.Versions {
            versions = append(versions, v)
        }
        sort.Slice(versions, func(i, j int) bool {
            return compareVersions(versions[i], versions[j]) > 0
        })
        for _, v := range versions {
            if versionMatches(v, version) {
                packageData = registryData.Versions[v]
                resolvedVersion = v
                break
            }
        }
        if resolvedVersion == "" {
            return InstallResult{
                Task:     task,
                Error:    fmt.Errorf("no matching version found for %s (tried %s)", version, strings.Join(versions, ", ")),
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
    if existingVersion, err := getInstalledVersion(packageDir); err == nil {
        if existingVersion == resolvedVersion {
            return InstallResult{
                Task:     task,
                Error:    nil,
                Duration: time.Since(startTime),
            }
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
func getAllVersions(versions map[string]Package) []string {
    result := make([]string, 0, len(versions))
    for v := range versions {
        result = append(result, v)
    }
    return result
}
func parseVersionConstraint(constraint string) (string, string, error) {
    if constraint == "" || constraint == "*" {
        return "", "", nil
    }
    if !strings.ContainsAny(constraint, "^~><= ") {
        return "=", constraint, nil
    }
    if strings.HasPrefix(constraint, "^") {
        return "^", strings.TrimPrefix(constraint, "^"), nil
    }
    if strings.HasPrefix(constraint, "~") {
        return "~", strings.TrimPrefix(constraint, "~"), nil
    }
    ops := []string{">=", "<=", ">", "<", "="}
    for _, op := range ops {
        if strings.HasPrefix(constraint, op) {
            return op, strings.TrimPrefix(constraint, op), nil
        }
    }
    if strings.Contains(constraint, " - ") {
        parts := strings.Split(constraint, " - ")
        if len(parts) == 2 {
            return "-", fmt.Sprintf("%s %s", parts[0], parts[1]), nil
        }
    }
    if strings.Contains(constraint, "x") {
        return "x", constraint, nil
    }
    return "", "", fmt.Errorf("unsupported version constraint: %s", constraint)
}
func satisfiesVersion(version, constraint string) bool {
    if constraint == "" || constraint == "*" {
        return true
    }
    op, ver, err := parseVersionConstraint(constraint)
    if err != nil {
        return false
    }
    switch op {
    case "":
        return version == ver
    case "=":
        return version == ver
    case "^":
        return satisfiesCaret(version, ver)
    case "~":
        return satisfiesTilde(version, ver)
    case ">":
        return compareVersions(version, ver) > 0
    case ">=":
        return compareVersions(version, ver) >= 0
    case "<":
        return compareVersions(version, ver) < 0
    case "<=":
        return compareVersions(version, ver) <= 0
    case "-":
        parts := strings.Split(ver, " ")
        if len(parts) != 2 {
            return false
        }
        return compareVersions(version, parts[0]) >= 0 &&
            compareVersions(version, parts[1]) <= 0
    case "x":
        pattern := strings.Replace(ver, "x", `\d+`, -1)
        pattern = strings.Replace(pattern, ".", `\.`, -1)
        pattern = "^" + pattern + "$"
        matched, _ := regexp.MatchString(pattern, version)
        return matched
    }
    return false
}
func satisfiesCaret(version, constraint string) bool {
    cv := strings.Split(constraint, ".")
    vv := strings.Split(version, ".")
    if len(cv) == 0 || len(vv) == 0 {
        return false
    }
    if cv[0] == "0" {
        if len(cv) > 1 && len(vv) > 1 {
            if cv[1] != vv[1] {
                return false
            }
        }
        if len(cv) > 2 && len(vv) > 2 {
            if cv[2] != vv[2] {
                return false
            }
        }
        return true
    }
    if cv[0] != vv[0] {
        return false
    }
    return compareVersions(version, constraint) >= 0
}
func satisfiesTilde(version, constraint string) bool {
    cv := strings.Split(constraint, ".")
    vv := strings.Split(version, ".")
    if len(cv) == 0 || len(vv) == 0 {
        return false
    }
    if len(cv) >= 3 && len(vv) >= 3 {
        if cv[0] != vv[0] || cv[1] != vv[1] {
            return false
        }
        return compareVersions(version, constraint) >= 0
    }
    if len(cv) >= 2 && len(vv) >= 2 {
        if cv[0] != vv[0] {
            return false
        }
        return compareVersions(version, constraint) >= 0
    }
    return cv[0] == vv[0]
}
func versionMatches(version, constraint string) bool {
	if _, err := strconv.Atoi(constraint); err == nil {
        return strings.HasPrefix(version, constraint+".")
    }
    if strings.HasPrefix(constraint, "npm:") {
        parts := strings.Split(constraint, "@")
        if len(parts) >= 2 {
            constraint = parts[len(parts)-1]
        }
    }
    orConstraints := strings.Split(constraint, "||")
    for _, c := range orConstraints {
        c = strings.TrimSpace(c)
        if satisfiesVersion(version, c) {
            return true
        }
    }
    return false
}
func compareVersions(v1, v2 string) int {
    parts1 := strings.Split(strings.TrimPrefix(v1, "v"), ".")
    parts2 := strings.Split(strings.TrimPrefix(v2, "v"), ".")
    prerelease1 := ""
    prerelease2 := ""
    if len(parts1) > 0 {
        if idx := strings.Index(parts1[len(parts1)-1], "-"); idx != -1 {
            prerelease1 = parts1[len(parts1)-1][idx+1:]
            parts1[len(parts1)-1] = parts1[len(parts1)-1][:idx]
        }
    }
    if len(parts2) > 0 {
        if idx := strings.Index(parts2[len(parts2)-1], "-"); idx != -1 {
            prerelease2 = parts2[len(parts2)-1][idx+1:]
            parts2[len(parts2)-1] = parts2[len(parts2)-1][:idx]
        }
    }
    maxLen := max(len(parts1), len(parts2))
    for i := 0; i < maxLen; i++ {
        var p1, p2 string
        if i < len(parts1) {
            p1 = parts1[i]
        }
        if i < len(parts2) {
            p2 = parts2[i]
        }
        n1, err1 := strconv.Atoi(p1)
        n2, err2 := strconv.Atoi(p2)
        if err1 == nil && err2 == nil {
            if n1 < n2 {
                return -1
            } else if n1 > n2 {
                return 1
            }
            continue
        }
        if p1 < p2 {
            return -1
        } else if p1 > p2 {
            return 1
        }
    }
    if prerelease1 != "" || prerelease2 != "" {
        if prerelease1 == "" {
            return 1
        }
        if prerelease2 == "" {
            return -1
        }
        if prerelease1 < prerelease2 {
            return -1
        } else if prerelease1 > prerelease2 {
            return 1
        }
    }
    return 0
}
func getInstalledVersion(packageDir string) (string, error) {
    packageJSONPath := filepath.Join(packageDir, "package.json")
    data, err := os.ReadFile(packageJSONPath)
    if err != nil {
        return "", err
    }
    var pkg struct {
        Version string `json:"version"`
    }
    if err := json.Unmarshal(data, &pkg); err != nil {
        return "", err
    }
    return pkg.Version, nil
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
func listGlobalPackages() {
    ui.Header("globally installed packages")
    globalDir, err := getGlobalInstallDir()
    if err != nil {
        ui.Error(fmt.Sprintf("failed to determine global directory: %v", err))
        return
    }
    if _, err := os.Stat(globalDir); os.IsNotExist(err) {
        ui.Warning("no global packages installed")
        return
    }
    entries, err := os.ReadDir(globalDir)
    if err != nil {
        ui.Error(fmt.Sprintf("error reading global packages: %v", err))
        return
    }
    packages := make([]string, 0)
    for _, entry := range entries {
        if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
            packageJSONPath := filepath.Join(globalDir, entry.Name(), "package.json")
            if packageData, err := readPackageJSONFromPath(packageJSONPath); err == nil {
                packages = append(packages, fmt.Sprintf("%s@%s", packageData.Name, packageData.Version))
            } else {
                packages = append(packages, entry.Name())
            }
        }
    }
    sort.Strings(packages)
    ui.Info(fmt.Sprintf("found %d global packages:", len(packages)))
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
func max(a, b int) int {
    if a > b {
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
