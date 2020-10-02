package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

var (
	HacktoberfestStartDate = time.Date(time.Now().Year(), time.October, 1, 0, 0, 0, 0, time.UTC)
	SpamLabel              = "Spam"
	ClosedState            = "closed"
)

func main() {
	if lbl := os.Getenv("SPAM_LABEL"); lbl != "" {
		SpamLabel = lbl
	}

	var token string
	switch {
	case os.Getenv("ACTIONS_RUNTIME_TOKEN") != "":
		token = os.Getenv("ACTIONS_RUNTIME_TOKEN")
	case os.Getenv("GITHUB_TOKEN") != "":
		token = os.Getenv("GITHUB_TOKEN")
	case os.Getenv("ACCESS_TOKEN") != "":
		token = os.Getenv("ACCESS_TOKEN")
	default:
		log.Fatalln("missing token")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	owner, repo, err := SplitRepositoryName(os.Getenv("GITHUB_REPOSITORY"))
	if err != nil {
		log.Fatalln(err)
	}

	prNum, err := ParsePullRequestNumber(os.Getenv("GITHUB_REF"))
	if err != nil {
		log.Fatalln(err)
	}

	client := github.NewClient(tc)

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, prNum)
	if err != nil {
		log.Fatalf("could not fetch the PR %d in %s/%s: %s", prNum, owner, repo, err)
	}

	u := pr.GetUser()
	u, _, err = client.Users.Get(ctx, u.GetLogin())
	if err != nil {
		log.Fatalf("could not fetch user %s: %s", u.GetLogin(), err)
	}

	repos, _, err := client.Repositories.List(ctx, u.GetLogin(), nil)
	if err != nil {
		log.Fatalf("could not fetch %s's repositories: %s", u.GetLogin(), err)
	}

	var forksCount int
	for _, repo := range repos {
		if repo.GetFork() {
			forksCount++
		}
	}

	files, _, err := client.PullRequests.ListFiles(ctx, owner, repo, prNum, nil)
	if err != nil {
		log.Fatalf("could not fetch the list of changed files in %s/%s#d: %w", owner, repo, prNum, err)
	}

	docsOnly := true
	for _, f := range files {
		docsOnly = docsOnly && strings.HasSuffix(f.GetFilename(), ".md")
	}

	if u.GetCreatedAt().After(HacktoberfestStartDate.Add(-24*time.Hour)) && len(repos) == forksCount {
		log.Printf("%s/%s#%d: user registered less than one day before Hacktoberfest and has only forked repositories", owner, repo, prNum)
		MarkAsSpam(ctx, owner, repo, prNum, client)
		return
	}

	if (len(files) == 1 || docsOnly) && pr.GetAdditions()+pr.GetDeletions() < 11 {
		log.Printf("%s/%s#%d: pull request has few changes either in a single file or only in documentation", owner, repo, prNum)
		MarkAsSpam(ctx, owner, repo, prNum, client)
		return
	}

	if len(files) == 1 && (pr.GetAdditions() == 0 || pr.GetDeletions() == 0) {
		log.Printf("%s/%s#%d: only one file changed with either additions or delitions only", owner, repo, prNum)
		MarkAsSpam(ctx, owner, repo, prNum, client)
		return
	}
}

func SplitRepositoryName(repo string) (string, string, error) {
	kv := strings.SplitN(repo, "/", 2)
	if len(kv) != 2 {
		return "", "", fmt.Errorf("invalid repo name %s", repo)
	}

	return kv[0], kv[1], nil
}

func ParsePullRequestNumber(ref string) (int, error) {
	s := strings.TrimPrefix(strings.TrimSuffix(ref, "/merge"), "refs/pull/")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid pull request ref %s", ref)
	}

	return n, nil
}

func MarkAsSpam(ctx context.Context, owner, repo string, num int, client *github.Client) error {
	return nil

	_, _, err := client.Issues.AddLabelsToIssue(ctx, owner, repo, num, []string{SpamLabel})
	if err != nil {
		return fmt.Errorf("failed to mark pull request as spam: %w", err)
	}

	log.Printf("marked %s/%s#%d as spam", owner, repo, num)

	if close := os.Getenv("CLOSE_SPAM_PRS"); close != "yes" {
		return nil
	}

	_, _, err = client.PullRequests.Edit(ctx, owner, repo, num, &github.PullRequest{
		State: &ClosedState,
	})

	log.Printf("closed %s/%s#%d", owner, repo, num)

	return nil
}
