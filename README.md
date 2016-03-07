# Glide vendor cleaner

## Important Note!!! ##
Before using this tool be sure that cleaning and commiting vendored directories to VCS does not violate the licenses of the packages you're vendoring.

For a detailed explanation on why Glide doesn't do this see [here](http://engineeredweb.com/blog/2016/go-why-not-strip-unused-pkgs/)


## Description

This tool will help you removing from the project vendor directories all the files not needed for building your project. By default it'll keep only `.go` (but not the tests) files of the packages listed in the `glide.lock` file.
If you want to also keep the test files of the needed packages you can provide the `--keep-tests` option.
If you want to keep all the files of the needed packages you can provide the `--keep-all` option.


## Build

Install glide.

`glide install` or `glide install --cache` or `glide install --cache-gopath` etc... (as you are more accustomed)

`go build` or `go install`

## Run
```
glide vendor cleaner

Usage:
  glide-vc [flags]

Flags:
      --dryrun       just output what will be removed
      --keep-all     keep all files of needed packages (instead of keeping only not test .go files)
      --keep-tests   keep also go test files
```

You have to run `glide-vc`, or (if glide is installed) `glide vc` inside your current project directory.

To see what it'll do use the `--dryrun` option.

### Example.

Tests removal of all unneeded packages keeping only the .go (also removing tests) files.

```
glide-vc --dryrun
```

Do it

```
glide-vc
```


Remove all the unneeded packages but keep all files for the needed ones:

```
glide-vc --keep-all
```

Remove all the unneeded packages but keep the go and test files for the needed ones:

```
glide-vc --keep-tests
```
