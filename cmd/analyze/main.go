package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var HacktoberfestStartDate = time.Date(time.Now().Year(), time.October, 1, 0, 0, 0, 0, time.UTC)

func main() {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	var stats []struct {
		Additions, Deletions int
		Files                int
		URL                  string
	}
	for i := 0; i < 4; i++ {
		results, _, err := client.Search.Issues(
			ctx,
			"label:spam",
			&github.SearchOptions{
				Sort:        "created",
				Order:       "desc",
				ListOptions: github.ListOptions{PerPage: 100, Page: i},
			},
		)
		if err != nil {
			log.Printf("failed to fetch page $d: %s", i, err)
			continue
		}

		for _, res := range results.Issues {
			if res.CreatedAt.Before(HacktoberfestStartDate) {
				break
			}

			if res.GetPullRequestLinks() == nil {
				continue
			}

			fullName := strings.SplitN(strings.TrimPrefix(res.GetHTMLURL(), "https://github.com/"), "/", 3)
			if len(fullName) != 3 {
				log.Println("malformed PR URL: %s", res.GetHTMLURL())
				continue
			}

			pr, _, err := client.PullRequests.Get(ctx, fullName[0], fullName[1], res.GetNumber())
			if err != nil {
				log.Println("failed to fetch %s#%d: %s", res.GetRepository().GetFullName(), res.GetNumber(), err)
				continue
			}

			files, _, err := client.PullRequests.ListFiles(ctx, fullName[0], fullName[1], res.GetNumber(), nil)
			if err != nil {
				log.Println("failed to fetch files for %s#%d: %s", res.GetRepository().GetFullName(), res.GetNumber(), err)
				continue
			}

			stats = append(stats, struct {
				Additions, Deletions, Files int
				URL                         string
			}{pr.GetAdditions(), pr.GetDeletions(), len(files), res.GetHTMLURL()})
		}
	}

	fmt.Println("add,del,files,url")
	for _, stat := range stats {
		fmt.Printf("%d,%d,%d,%s\n", stat.Additions, stat.Deletions, stat.Files, stat.URL)
	}

	log.Printf("analyzed %d pull requests", len(stats))
}
