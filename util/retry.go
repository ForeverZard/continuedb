package util

import (
	"time"
	"math"
	"math/rand"
)

// RetryOption is the set of arguments to control a Retry.
// On every retry, Retry will wait min(InitBackoff * Base^attemptsCount * (1 - RandomFactor), MaxBackoff) ms.
type RetryOption struct {
	InitBackoff time.Duration
	Base float64
	RandomFactor float64
	MaxBackoff time.Duration
	// 0 for infinity.
	MaxAttempts int
	// Stopper is a optional argument to stop retry.
	Stopper <-chan struct{}
}

func (opts *RetryOption) initDefault() {
	if opts.InitBackoff <= 0 {
		opts.InitBackoff = 50
	}
	if opts.Base <= 0 {
		opts.Base = 2
	}
	if opts.RandomFactor <= 0 {
		opts.RandomFactor = 0.12
	}
}

// Retry is the helper that implement expontial retry backoff algorithm.
type Retry struct {
	opts *RetryOption
	attemptsCount int
	isReset bool
}

// NewRetry returns a Retry with the given opts.
func NewRetry(opts *RetryOption) *Retry {
	opts.initDefault()
	return &Retry{opts: opts}
}

func (r *Retry) Reset() {
	r.attemptsCount = 0
	r.isReset = true
}

func (r *Retry) waitTime() time.Duration {
	backoff := float64(r.opts.InitBackoff) * math.Pow(r.opts.Base, float64(r.attemptsCount))
	if backoff > float64(r.opts.MaxBackoff) {
		backoff = float64(r.opts.MaxBackoff)
	}
	delta := backoff * r.opts.RandomFactor
	// Note: time.Duration is a int64
	return time.Duration(backoff - delta + rand.Float64() * (2*delta+1))
}

// Next check whether the retry should go and make the current retry.
func (r *Retry) Next() bool {
	if r.isReset {
		r.isReset = false
		return true
	}

	if r.opts.MaxAttempts != 0 && r.attemptsCount >= r.opts.MaxAttempts {
		return false
	}

	select {
	case <-time.After(r.waitTime()):
		r.attemptsCount++
		return true
	case <-r.opts.Stopper:
		return false
	}
}
