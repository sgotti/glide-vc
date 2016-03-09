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
	dryrun  bool
	onlyGo  bool
	noTests bool
}

var opts options

func init() {
	cmd.PersistentFlags().BoolVar(&opts.dryrun, "dryrun", false, "just output what will be removed")
	cmd.PersistentFlags().BoolVar(&opts.onlyGo, "only-go", false, "keep only go files (including go test files)")
	cmd.PersistentFlags().BoolVar(&opts.noTests, "no-tests", false, "remove also go test files (requires --only-go)")
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
	// TODO(sgotti) Should we also consider devImports?
	for _, imp := range lock.Imports {
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

	var searchPath string
	var markForDelete []struct {
		path  string
		isDir bool
	}

	fn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == searchPath || path == vpath {
			return nil
		}

		localPath := strings.TrimPrefix(path, searchPath)
		keep := false

		// If the file's parent directory is a needed package, keep it.
		for _, name := range pkgList {
			if !info.IsDir() && filepath.Dir(localPath) == name {
				if opts.onlyGo {
					if opts.noTests {
						if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
							keep = true
						}
					} else {
						if strings.HasSuffix(path, ".go") {
							keep = true
						}
					}
				} else {
					keep = true
				}
			}
		}

		// If a directory is a needed package or a parent then keep it
		if keep == false && info.IsDir() {
			for _, name := range pkgList {
				if strings.HasPrefix(name, localPath) {
					keep = true
				}
			}
		}

		// Avoid marking for removal childs of already marked directories
		for _, marked := range markForDelete {
			if marked.isDir {
				if strings.HasPrefix(filepath.Dir(path), marked.path) {
					return nil
				}
			}
		}

		if keep == false {
			// Mark for deletion
			markForDelete = append(markForDelete, struct {
				path  string
				isDir bool
			}{path, info.IsDir()})
		}

		return nil
	}

	// Walk vendor directory
	searchPath = vpath + string(os.PathSeparator)
	if err = filepath.Walk(searchPath, fn); err != nil {
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
