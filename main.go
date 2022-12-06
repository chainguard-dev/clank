package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/sethvargo/ratchet/parser"
	"gopkg.in/yaml.v3"
)

func main() {
	ctx := context.Background()

	tmp, err := os.MkdirTemp("", "clank-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	for _, arg := range os.Args[1:] {
		if err := filepath.Walk(arg,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				fmt.Println(path)
				table := tablewriter.NewWriter(os.Stdout)
				table.SetHeader([]string{"Ref", "Status", "Lines", "Details"})
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				details, err := handle(ctx, f, tmp)
				if err != nil {
					return err
				}

				for _, d := range details {
					if d.err == nil {
						table.Append([]string{d.ref, color.GreenString("OK"), fmt.Sprint(d.lines), ""})
					} else {
						table.Append([]string{d.ref, color.RedString("ERROR"), fmt.Sprint(d.lines), d.err.Error()})
					}
				}
				table.Render()
				fmt.Println()

				return nil
			}); err != nil {
			log.Fatal(err)
		}
	}
}

type details struct {
	ref   string
	lines []int
	err   error
}

func handle(ctx context.Context, r io.Reader, tmp string) ([]details, error) {
	n := new(yaml.Node)
	if err := yaml.NewDecoder(r).Decode(n); err != nil {
		return nil, err
	}

	parse := parser.Actions{}
	reflist, err := parse.Parse(n)
	if err != nil {
		return nil, err
	}

	out := make([]details, 0, len(reflist.All()))
	for ref, nodes := range reflist.All() {
		ref := ref

		if !strings.HasPrefix(ref, "actions://") {
			continue
		}

		s := strings.Split(strings.TrimPrefix(ref, "actions://"), "@")
		if len(s) != 2 {
			log.Printf("wanted len() = 2, got %v", s)
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
			err:   checkRepo(ctx, repo[0], repo[1], sha, tmp),
		})
	}
	return out, nil
}

func checkRepo(ctx context.Context, owner, repo, sha, basedir string) error {
	url := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	dir := filepath.Join(basedir, repo)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if out, err := exec.CommandContext(ctx, "git", "clone", "--filter=tree:0", url, dir).CombinedOutput(); err != nil {
			return fmt.Errorf("could not clone repo: %s", out)
		}
		if out, err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "remove", "origin").CombinedOutput(); err != nil {
			return fmt.Errorf("could not remove remote: %s", out)
		}
	}

	if out, err := exec.CommandContext(ctx, "git", "-C", dir, "cat-file", "-e", sha).CombinedOutput(); err != nil {
		log.Println("cat-file", url, dir, sha, string(out))
		return fmt.Errorf("SHA not present in repo")
	}

	return nil
}
