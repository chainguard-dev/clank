package main

import (
	"context"
	"fmt"
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

	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	n := new(yaml.Node)
	if err := yaml.NewDecoder(f).Decode(n); err != nil {
		log.Fatal(err)
	}

	parse := parser.Actions{}
	reflist, err := parse.Parse(n)
	if err != nil {
		log.Fatal(err)
	}

	tmp, err := os.MkdirTemp("", "backlash-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Ref", "Status", "Lines", "Details"})

	for ref, nodes := range reflist.All() {
		ref := ref
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
		log.Println(repo, sha)

		if err := checkRepo(ctx, repo[0], repo[1], sha, tmp); err == nil {
			table.Append([]string{ref, color.GreenString("OK"), "", ""})
		} else {
			fmt.Println(repo, sha, err)
			table.Append([]string{ref, color.RedString("ERROR"), fmt.Sprint(lines), err.Error()})
		}
	}

	table.Render()
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
