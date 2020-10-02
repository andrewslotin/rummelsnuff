Rummelsnuff
===========

A GitHub action to mark and close spam PRs created to get a free HacktoberFest T-shirt.

Rules
-----

A pull request is considered as spam in following cases:

* The author has registered after this year's Hacktoberfest and has only forked repositories
* The PR is changing documentation insignificantly
* The PR consists of additions or deletions only

Configuration
-------------

The action needs an access token to manage PRs. To provide an access token, add `access_token: ${{ secrets.GITHUB_TOKEN }}` to the `with:` section of your workflow step (see example below).

By default this action adds "Spam" label and closes the PR that is recognized as spam. A custom label can be provided via the `spam_label` input. To disable closing PRs set the `close_spam_prs` to any value except `"yes"`, for example:

``` yaml
- name: Rummelsnuff
  uses: andrewslotin/rummelsnuff@master
    with:
      spam_label: "Bad PR" # default: "Spam"
      close_spam_prs: "no" # default: "yes"
      access_token: ${{ secrets.GITHUB_TOKEN }} # one-time access token generated for this action run
```
