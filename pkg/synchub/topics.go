package synchub

import "fmt"

// A2A topic builders. Centralized here so a typo can't sneak in via a
// stray string literal elsewhere in the package.

func topicQAOpened(session string) string   { return fmt.Sprintf("synchub.%s.qa.opened", session) }
func topicQAAnswered(session string) string { return fmt.Sprintf("synchub.%s.qa.answered", session) }
func topicQAExpired(session string) string  { return fmt.Sprintf("synchub.%s.qa.expired", session) }

func topicLockAcquired(session string) string { return fmt.Sprintf("synchub.%s.lock.acquired", session) }
func topicLockReleased(session string) string { return fmt.Sprintf("synchub.%s.lock.released", session) }
func topicLockTimedOut(session string) string { return fmt.Sprintf("synchub.%s.lock.timed-out", session) }
