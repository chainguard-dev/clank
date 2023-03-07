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
		dir := arg

		if strings.HasPrefix(arg, "https://") {
			s := strings.Split(arg, "/")
			cloneDir := filepath.Join(tmp, s[3], s[4])

			if out, err := exec.CommandContext(ctx, "git", "clone", "--depth", "1", arg, cloneDir).CombinedOutput(); err != nil {
				log.Fatalf("could not clone repo: %s %s", err, string(out))
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
	dir := filepath.Join(basedir, owner, repo)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Disable prompting for password
		os.Setenv("GIT_TERMINAL_PROMPT", "false")

		var cloneErr error
		for _, url := range []string{
			// https
			fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
			// ssh
			fmt.Sprintf("git@github.com:%s/%s.git", owner, repo),
		} {
			cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", "--filter=tree:0", url, dir)
			out, err := cmd.CombinedOutput()
			if err == nil {
				cloneErr = nil
				break
			} else {
				cloneErr = fmt.Errorf("could not clone repo: %s", out)
			}
		}
		if cloneErr != nil {
			return cloneErr
		}
	}

	for _, ref := range []string{"branch", "tag"} {
		cmd := exec.CommandContext(ctx, "git", "-C", dir, ref, "--contains", sha)
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("SHA not present in repo")
		}
		if len(out) > 0 {
			return nil
		}
	}

	return fmt.Errorf("SHA not present in repo")
}
