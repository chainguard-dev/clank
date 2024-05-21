// Copyright 2023 Chainguard, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	// Using banydonk/yaml instead of the default yaml pkg because the default
	// pkg incorrectly escapes unicode. https://github.com/go-yaml/yaml/issues/737
	// comment from https://github.com/sethvargo/ratchet/blob/main/parser/parser.go#L11
	"github.com/braydonk/yaml"
	"github.com/fatih/color"
	"github.com/google/go-github/v50/github"
	"github.com/olekukonko/tablewriter"
	"github.com/sethvargo/ratchet/parser"
	"golang.org/x/oauth2"
)

func main() {
	err := mainImpl()
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}
}

func mainImpl() error {
	ctx := context.Background()

	base := http.DefaultClient
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: t},
		)
		base = oauth2.NewClient(ctx, ts)
	}
	client := github.NewClient(base)
	tmp, err := os.MkdirTemp("", "clank-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	for _, arg := range os.Args[1:] {
		dir := arg

		// If user gives us a URL, fetch the workflow content.
		if strings.HasPrefix(arg, "https://") {
			s := strings.Split(arg, "/")
			owner := s[3]
			repo := strings.TrimSuffix(s[4], ".git")
			cloneDir := filepath.Join(tmp, owner, repo)

			// Make sure parent directory exists.
			if err := os.MkdirAll(filepath.Join(cloneDir, ".github/workflows"), 0700); err != nil && !os.IsExist(err) {
				return err
			}

			// Fetch workflow files.
			if err := getContent(ctx, client, owner, repo, cloneDir, ".github/workflows"); err != nil {
				return fmt.Errorf("could not get content: %w", err)
			}
			dir = filepath.Join(cloneDir, ".github", "workflows")
		}

		if err := filepath.Walk(dir,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
					return nil
				}
				fmt.Println(path)
				w, err := parseWorkflow(path)
				if err != nil {
					return fmt.Errorf("unable to parse workflow: %w", err)
				}
				if len(w.Refs()) == 0 {
					// No refs found, so nothing to check.
					return nil
				}

				details, err := handle(ctx, client, w)
				if err != nil {
					return err
				}

				// outErr is used to track if there was any error - this is used to make sure a non-zero exit code is returned.
				var outErr error
				table := tablewriter.NewWriter(os.Stdout)
				table.SetHeader([]string{"Ref", "Status", "Lines", "Details"})
				for _, d := range details {
					if d.err == nil {
						table.Append([]string{d.ref, color.GreenString("OK"), fmt.Sprint(d.lines), ""})
					} else {
						table.Append([]string{d.ref, color.RedString("ERROR"), fmt.Sprint(d.lines), d.err.Error()})
						outErr = fmt.Errorf("problem found with %s", path)
					}
				}
				table.Render()
				fmt.Println()

				return outErr
			}); err != nil {
			return err
		}
	}

	return nil
}

type details struct {
	ref   string
	lines []int
	err   error
}

func parseWorkflow(path string) (*parser.RefsList, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	n := new(yaml.Node)
	if err := yaml.NewDecoder(f).Decode(n); err != nil {
		return nil, err
	}

	parse := parser.Actions{}
	return parse.Parse([]*yaml.Node{n})
}

func handle(ctx context.Context, client *github.Client, workflow *parser.RefsList) ([]details, error) {
	cache := &containsCache{
		cache: make(map[commitKey]bool),
	}

	out := make([]details, 0, len(workflow.All()))
	for ref, nodes := range workflow.All() {
		ref := ref

		if !strings.HasPrefix(ref, "actions://") {
			continue
		}

		s := strings.Split(strings.TrimPrefix(ref, "actions://"), "@")
		if len(s) != 2 {
			return nil, fmt.Errorf("unexpected ref: %s", ref)
		}
		sha := s[1]
		repo := strings.Split(s[0], "/")

		lines := []int{}
		for _, n := range nodes {
			lines = append(lines, n.Line)
		}

		out = append(out, details{
			ref:   ref,
			lines: lines,
			err:   checkRepo(ctx, client, cache, repo[0], repo[1], sha),
		})
	}
	return out, nil
}

func getContent(ctx context.Context, client *github.Client, owner, repo, localPath, targetPath string) error {
	file, dir, _, err := client.Repositories.GetContents(ctx, owner, repo, targetPath, &github.RepositoryContentGetOptions{})
	if err != nil {
		return err
	}

	// Content returned could be a folder or a single file.

	// If file, write the content to disk.
	if file != nil {
		data, err := file.GetContent()
		if err != nil {
			return err
		}

		return os.WriteFile(filepath.Join(localPath, file.GetPath()), []byte(data), 0o600)
	}

	// If directory, recursively traverse.
	for _, d := range dir {
		if err := getContent(ctx, client, owner, repo, localPath, d.GetPath()); err != nil {
			return err
		}
	}

	return nil
}

func checkRepo(ctx context.Context, client *github.Client, cache *containsCache, owner, repo, sha string) error {
	ok, err := cache.Contains(ctx, client, owner, repo, sha)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("SHA not present in repo")
	}
	return nil
}

type commitKey struct {
	owner, repo, sha string
}

// containsCache caches response values for whether a commit is contained in a given repo.
// This allows us to deduplicate work if we've already checked this commit.
type containsCache struct {
	cache map[commitKey]bool
}

func (c *containsCache) Contains(ctx context.Context, client *github.Client, owner, repo, sha string) (bool, error) {
	key := commitKey{
		owner: owner,
		repo:  repo,
		sha:   sha,
	}
	v, ok := c.cache[key]
	if ok {
		return v, nil
	}

	out, err := checkImposterCommit(ctx, client, owner, repo, sha)
	c.cache[key] = out
	return out, err
}

func checkImposterCommit(ctx context.Context, c *github.Client, owner, repo, target string) (bool, error) {
	branches, _, err := c.Repositories.ListBranches(ctx, owner, repo, &github.BranchListOptions{})
	if err != nil {
		return false, err
	}
	for _, b := range branches {
		ok, err := refContains(ctx, c, owner, repo, fmt.Sprintf("refs/heads/%s", b.GetName()), target)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}

	tags, _, err := c.Repositories.ListTags(ctx, owner, repo, &github.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, t := range tags {
		ok, err := refContains(ctx, c, owner, repo, fmt.Sprintf("refs/tags/%s", t.GetName()), target)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}

	return false, nil
}

func refContains(ctx context.Context, c *github.Client, owner, repo, base, target string) (bool, error) {
	diff, resp, err := c.Repositories.CompareCommits(ctx, owner, repo, base, target, &github.ListOptions{PerPage: 1})
	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			// NotFound can be returned for some divergent cases: "404 No common ancestor between ..."
			return false, nil
		}
		return false, fmt.Errorf("error comparing revisions: %w", err)
	}

	// Target should be behind or at the base ref if it is considered contained.
	return diff.GetStatus() == "behind" || diff.GetStatus() == "identical", nil
}
