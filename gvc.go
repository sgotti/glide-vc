package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/glide/cfg"
	gpath "github.com/Masterminds/glide/path"
	"github.com/spf13/cobra"
)

var cmd = &cobra.Command{
	Use:   "glide-vc",
	Short: "glide vendor cleaner",
	Run:   glidevc,
}

type options struct {
	dryrun       bool
	onlyGo       bool
	noTests      bool
	noLegalFiles bool
}

var opts options

func init() {
	cmd.PersistentFlags().BoolVar(&opts.dryrun, "dryrun", false, "just output what will be removed")
	cmd.PersistentFlags().BoolVar(&opts.onlyGo, "only-go", false, "keep only go files (including go test files)")
	cmd.PersistentFlags().BoolVar(&opts.noTests, "no-tests", false, "remove also go test files (requires --only-go)")
	cmd.PersistentFlags().BoolVar(&opts.noLegalFiles, "no-legal-files", false, "remove also licenses and legal files")
}

func main() {
	cmd.Execute()
}

func glidevc(cmd *cobra.Command, args []string) {
	if opts.noTests && !opts.onlyGo {
		fmt.Printf("--no-tests requires --only-go")
		os.Exit(1)
	}

	if err := cleanup(".", opts); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
	return
}

func cleanup(path string, opts options) error {
	lock, err := LoadGlideLockfile(path)
	if err != nil {
		return fmt.Errorf("Could not load lockfile: %v", err)
	}

	// The package list already have the path converted to the os specific
	// path separator, needed for future comparisons.
	pkgList := []string{}
	repoList := []string{}
	// TODO(sgotti) Should we also consider devImports?
	for _, imp := range lock.Imports {
		// This converts pkg separator "/" to os specific separator
		repoList = append(repoList, filepath.FromSlash(imp.Name))

		if len(imp.Subpackages) > 0 {
			for _, sp := range imp.Subpackages {
				// This converts pkg separator "/" to os specific separator
				pkgList = append(pkgList, filepath.Join(imp.Name, sp))
			}
		}
		// TODO(sgotti) we cannot skip the base import if it has subpackages
		// because glide doesn't write "." as a subpackage, otherwise if some
		// files in the base import are needed they will be removed.

		// This converts pkg separator "/" to os specific separator
		pkgList = append(pkgList, filepath.FromSlash(imp.Name))
	}

	vpath, err := gpath.Vendor()
	if err != nil {
		return err
	}
	if vpath == "" {
		return fmt.Errorf("cannot fine vendor dir")
	}

	type pathData struct {
		path  string
		isDir bool
	}
	var searchPath string
	markForKeep := map[string]pathData{}
	markForDelete := []pathData{}

	// Walk vendor directory
	searchPath = vpath + string(os.PathSeparator)
	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == searchPath || path == vpath {
			return nil
		}

		localPath := strings.TrimPrefix(path, searchPath)

		lastVendorPath, err := getLastVendorPath(localPath)
		if err != nil {
			return err
		}
		if lastVendorPath == "" {
			lastVendorPath = localPath
		}

		keep := false
		for _, name := range pkgList {
			// If the file's parent directory is a needed package, keep it.
			if !info.IsDir() && filepath.Dir(lastVendorPath) == name {
				if opts.onlyGo {
					validGoSuffix := strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".s") // also include assembly files

					if opts.noTests {
						if validGoSuffix && !strings.HasSuffix(path, "_test.go") {
							keep = true
						}
					} else {
						if validGoSuffix {
							keep = true
						}
					}
				} else {
					keep = true
				}
			}
		}

		// Keep all the legal files inside top repo dir and required package dirs
		for _, name := range append(repoList, pkgList...) {
			if !info.IsDir() && filepath.Dir(lastVendorPath) == name {
				if !opts.noLegalFiles {
					if IsLegalFile(path) {
						keep = true
					}
				}
			}
		}

		// If a directory is a needed package then keep it
		if keep == false && info.IsDir() {
			for _, name := range pkgList {
				if name == lastVendorPath {
					keep = true
				}
			}
		}

		if keep {
			// Keep also all parents of current path
			curpath := localPath
			for {
				curpath = filepath.Dir(curpath)
				if curpath == "." {
					break
				}
				if _, ok := markForKeep[curpath]; ok {
					// Already marked for keep
					break
				}
				markForKeep[curpath] = pathData{curpath, true}
			}

			// Mark for keep
			markForKeep[localPath] = pathData{localPath, info.IsDir()}
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Generate deletion list
	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Ignore not existant files due to previous removal of the parent directory
			if !os.IsNotExist(err) {
				return err
			}
		}
		localPath := strings.TrimPrefix(path, searchPath)
		if localPath == "" {
			return nil
		}
		if _, ok := markForKeep[localPath]; !ok {
			markForDelete = append(markForDelete, pathData{path, info.IsDir()})
			if info.IsDir() {
				// skip directory contents since it has been marked for removal
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Perform the actual delete.
	for _, marked := range markForDelete {
		localPath := strings.TrimPrefix(marked.path, searchPath)
		if marked.isDir {
			fmt.Printf("Removing unused dir: %s\n", localPath)
		} else {
			fmt.Printf("Removing unused file: %s\n", localPath)
		}
		if !opts.dryrun {
			rerr := os.RemoveAll(marked.path)
			if rerr != nil {
				return rerr
			}
		}
	}

	return nil
}

func getLastVendorPath(path string) (string, error) {
	curpath := path
	for {
		if curpath == "." {
			return "", nil
		}
		if filepath.Base(curpath) == "vendor" {
			return filepath.Rel(curpath, path)
		}
		curpath = filepath.Dir(curpath)
	}
}

// LoadLockfile loads the contents of a glide.lock file.
func LoadGlideLockfile(base string) (*cfg.Lockfile, error) {
	yml, err := ioutil.ReadFile(filepath.Join(base, gpath.LockFile))
	if err != nil {
		return nil, err
	}
	lock, err := cfg.LockfileFromYaml(yml)
	if err != nil {
		return nil, err
	}

	return lock, nil
}

// File lists and code took from https://github.com/client9/gosupplychain/blob/master/license.go

// LicenseFilePrefix is a list of filename prefixes that indicate it
//  might contain a software license
var LicenseFilePrefix = []string{
	"licence", // UK spelling
	"license", // US spelling
	"copying",
	"unlicense",
	"copyright",
	"copyleft",
}

// LegalFileSubstring are substrings that indicate the file is likely
// to contain some type of legal declaration.  "legal" is often used
// that it might moved to LicenseFilePrefix
var LegalFileSubstring = []string{
	"legal",
	"notice",
	"disclaimer",
	"patent",
	"third-party",
	"thirdparty",
}

// IsLegalFile returns true if the file is likely to contain some type
// of of legal declaration or licensing information
func IsLegalFile(path string) bool {
	lowerfile := strings.ToLower(filepath.Base(path))
	for _, prefix := range LicenseFilePrefix {
		if strings.HasPrefix(lowerfile, prefix) {
			return true
		}
	}
	for _, substring := range LegalFileSubstring {
		if strings.Index(lowerfile, substring) != -1 {
			return true
		}
	}
	return false
}
