package main

import (
	"context"
	"encoding/json"
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

type Event struct {
	Number      int                 `json:"number"`
	PullRequest *github.PullRequest `json:"pull_request"`
}

func main() {
	if lbl := os.Getenv("INPUT_SPAM_LABEL"); lbl != "" {
		SpamLabel = lbl
	}

	token := os.Getenv("INPUT_ACCESS_TOKEN")
	if token == "" {
		log.Fatalln("missing token")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	owner, repo, err := splitRepositoryName(os.Getenv("GITHUB_REPOSITORY"))
	if err != nil {
		log.Fatalln(err)
	}

	fd, err := os.Open(os.Getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		log.Fatalf("failed to read event data: %s", err)
	}

	var event Event
	if err := json.NewDecoder(fd).Decode(&event); err != nil {
		log.Fatalf("failed to decode event data: %s", err)
	}
	fd.Close()

	prNum, forked := event.Number, event.PullRequest.Head.Repo.GetFork()
	if !forked {
		fmt.Println("the pull request is not from a forked repository")
		os.Exit(0)
	}

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
		log.Fatalf("could not fetch the list of changed files in %s/%s#%d: %w", owner, repo, prNum, err)
	}

	docsOnly := true
	for _, f := range files {
		docsOnly = docsOnly && strings.HasSuffix(f.GetFilename(), ".md")
	}

	if u.GetCreatedAt().After(HacktoberfestStartDate.Add(-24*time.Hour)) && len(repos) == forksCount {
		fmt.Printf("::error %s/%s#%d: user registered less than one day before Hacktoberfest and has only forked repositories", owner, repo, prNum)
		if err := MarkAsSpam(ctx, owner, repo, prNum, client); err != nil {
			log.Printf("failed to close a spam pr: %s", err)
		}
		os.Exit(1)
	}

	if docsOnly && pr.GetAdditions()+pr.GetDeletions() < 10 {
		fmt.Printf("::error %s/%s#%d: pull request is only few changes in documentation", owner, repo, prNum)
		if err := MarkAsSpam(ctx, owner, repo, prNum, client); err != nil {
			log.Printf("failed to close a spam pr: %s", err)
		}
		os.Exit(1)
	}

	if len(files) == 1 && (pr.GetAdditions() == 0 || pr.GetDeletions() == 0) {
		fmt.Printf("::error %s/%s#%d: only one file changed with either additions or deletions only", owner, repo, prNum)
		if err := MarkAsSpam(ctx, owner, repo, prNum, client); err != nil {
			log.Printf("failed to close a spam pr: %s", err)
		}
		os.Exit(1)
	}
}

func splitRepositoryName(repo string) (string, string, error) {
	kv := strings.SplitN(repo, "/", 2)
	if len(kv) != 2 {
		return "", "", fmt.Errorf("invalid repo name %s", repo)
	}

	return kv[0], kv[1], nil
}

func parsePullRequestNumber(ref string) (int, error) {
	s := strings.TrimPrefix(strings.TrimSuffix(ref, "/merge"), "refs/pull/")
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid pull request ref %s", ref)
	}

	return n, nil
}

func getPullRequestNumber(ctx context.Context, owner, repo string, client *github.Client) (int, error) {
	if prNum, err := strconv.Atoi(os.Getenv("INPUT_PR_NUM")); err == nil {
		return prNum, nil
	}

	prNum, err := parsePullRequestNumber(os.Getenv("GITHUB_REF"))
	if err != nil {
		log.Printf("failed to find the pull request by ref: %s", err)

		prNum, err = GetPullRequestBySHA(ctx, owner, repo, os.Getenv("GITHUB_SHA"), client)
		if err != nil {
			return 0, fmt.Errorf("failed to find the PR by SHA: %s", err)
		}
	}

	return prNum, nil
}

func GetPullRequestBySHA(ctx context.Context, owner, repo, sha string, client *github.Client) (int, error) {
	prs, _, err := client.PullRequests.ListPullRequestsWithCommit(ctx, owner, repo, sha, &github.PullRequestListOptions{
		State: "open",
	})
	if err != nil {
		return 0, fmt.Errorf("could not search for pull requests: %w", err)
	}

	if len(prs) == 0 {
		return 0, fmt.Errorf("could not find a pull request containing %s", sha)
	}

	return prs[0].GetNumber(), nil
}

func MarkAsSpam(ctx context.Context, owner, repo string, num int, client *github.Client) error {
	_, _, err := client.Issues.AddLabelsToIssue(ctx, owner, repo, num, []string{SpamLabel})
	if err != nil {
		return fmt.Errorf("failed to mark pull request as spam: %w", err)
	}

	log.Printf("marked %s/%s#%d as spam", owner, repo, num)

	if close := os.Getenv("INPUT_CLOSE_SPAM_PRS"); close != "yes" {
		return nil
	}

	_, _, err = client.PullRequests.Edit(ctx, owner, repo, num, &github.PullRequest{
		State: &ClosedState,
	})

	log.Printf("closed %s/%s#%d", owner, repo, num)

	return nil
}
