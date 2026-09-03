package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/vbonnet/dear-agent/internal/agenticreview"
	"github.com/vbonnet/dear-agent/internal/mergeloop"
)

// reviewClockTimeout bounds the two extra GitHub reads the review clock costs
// per candidate pull request. They are ordinary REST reads on the same order as
// the required-check projection the loop already performs per pull request.
const reviewClockTimeout = 60 * time.Second

// loadReviewPolicy reads the per-family review policy that gates merges.
//
// An absent file at the default path leaves the gate off, because the loop can
// be pointed at repositories that have not adopted it. An unreadable or invalid
// file is a hard error either way: a policy that cannot be parsed must never
// quietly become no policy, which is the failure mode where a gate looks
// installed and enforces nothing.
func loadReviewPolicy(path string, explicit bool) (*agenticreview.Config, error) {
	cfg, err := agenticreview.LoadConfig(path)
	if err == nil {
		return &cfg, nil
	}
	if !explicit && errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return nil, err
}

// enrichReviewClock attaches the timing the review gate needs to age a silent
// reviewer family out: when each current label was applied, and when this head
// became reviewable.
//
// It is deliberately the same derivation the required status check performs
// (agenticreview.Clock), so the loop and the check can never disagree about
// which reviewers have run out of time.
func enrichReviewClock(ctx context.Context, repo string, pr *mergeloop.PR, now time.Time) error {
	pr.ObservedAt = now

	var events []agenticreview.TimelineEvent
	if err := ghDecode(ctx, &events, "api", "--paginate",
		fmt.Sprintf("repos/%s/issues/%d/timeline", repo, pr.Number)); err != nil {
		return fmt.Errorf("reading review timeline: %w", err)
	}

	var head struct {
		Commit struct {
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := ghDecode(ctx, &head, "api", fmt.Sprintf("repos/%s/commits/%s", repo, pr.HeadSHA)); err != nil {
		return fmt.Errorf("reading head commit: %w", err)
	}

	pr.LabelAppliedAt, pr.ReadyAt = agenticreview.Clock(events, agenticreview.Head{
		Labels:      pr.Labels,
		CreatedAt:   pr.CreatedAt,
		CommittedAt: head.Commit.Committer.Date,
		IsDraft:     pr.IsDraft,
	})
	return nil
}

func ghDecode(ctx context.Context, into any, args ...string) error {
	out, err := ghJSON(ctx, reviewClockTimeout, args)
	if err != nil {
		return err
	}
	return json.Unmarshal(out, into)
}
