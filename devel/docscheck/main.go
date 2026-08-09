// Command docscheck verifies that relative links in repository Markdown files
// resolve to files or directories in the checkout.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	inlineLink    = regexp.MustCompile(`!?\[[^]]*\]\(([^)]+)\)`)
	referenceLink = regexp.MustCompile(`^\s*\[[^]]+\]:\s*(\S+)`)
)

func main() {
	root := flag.String("root", ".", "repository root to check")
	flag.Parse()

	problems, err := check(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(problems) != 0 {
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, problem)
		}
		os.Exit(1)
	}
	fmt.Println("relative documentation links resolve")
}

func check(root string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}

	var files []string
	err = filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && path != absRoot && skippedDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk documentation: %w", err)
	}
	sort.Strings(files)

	var problems []string
	for _, path := range files {
		fileProblems, checkErr := checkFile(absRoot, path)
		if checkErr != nil {
			return nil, checkErr
		}
		problems = append(problems, fileProblems...)
	}
	return problems, nil
}

func skippedDirectory(name string) bool {
	switch name {
	case ".git", "bin", "dist", "site-build", "vendor":
		return true
	default:
		return false
	}
}

func checkFile(root, path string) ([]string, error) {
	// #nosec G304 -- path comes from WalkDir beneath the selected repository root.
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	relFile, err := filepath.Rel(root, path)
	if err != nil {
		return nil, fmt.Errorf("relativize %s: %w", path, err)
	}
	return checkContent(root, path, relFile, content)
}

func checkContent(root, path, relFile string, content []byte) ([]string, error) {
	var problems []string
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNumber := 0
	inFence := false
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		for _, destination := range destinations(line) {
			resolved, local := resolveDestination(root, filepath.Dir(path), destination)
			if !local {
				continue
			}
			if _, statErr := os.Stat(resolved); statErr != nil {
				if errors.Is(statErr, os.ErrNotExist) {
					problems = append(problems, fmt.Sprintf("%s:%d: relative link %q does not resolve", filepath.ToSlash(relFile), lineNumber, destination))
					continue
				}
				return nil, fmt.Errorf("inspect link target %s: %w", resolved, statErr)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return problems, nil
}

func destinations(line string) []string {
	var result []string
	for _, match := range inlineLink.FindAllStringSubmatch(line, -1) {
		if len(match) == 2 {
			result = append(result, markdownDestination(match[1]))
		}
	}
	if match := referenceLink.FindStringSubmatch(line); len(match) == 2 {
		result = append(result, markdownDestination(match[1]))
	}
	return result
}

func markdownDestination(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "<") {
		if end := strings.Index(value, ">"); end >= 0 {
			return value[1:end]
		}
	}
	if index := strings.IndexAny(value, " \t"); index >= 0 {
		return value[:index]
	}
	return value
}

func resolveDestination(root, sourceDirectory, destination string) (string, bool) {
	if destination == "" || strings.HasPrefix(destination, "#") || strings.HasPrefix(destination, "//") {
		return "", false
	}
	parsed, err := url.Parse(destination)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return "", false
	}
	linkPath, err := url.PathUnescape(parsed.Path)
	if err != nil || linkPath == "" {
		return "", false
	}
	if path.IsAbs(linkPath) {
		return filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(linkPath, "/"))), true
	}
	return filepath.Join(sourceDirectory, filepath.FromSlash(linkPath)), true
}
