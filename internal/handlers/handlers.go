package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/podushkina/taskqueue/internal/task"
)

func Echo(ctx context.Context, t *task.Task) (string, error) {
	time.Sleep(1 * time.Second)
	return fmt.Sprintf("echo: %s", t.Payload), nil
}

func Reverse(ctx context.Context, t *task.Task) (string, error) {
	time.Sleep(500 * time.Millisecond)
	runes := []rune(t.Payload)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes), nil
}

func Sum(ctx context.Context, t *task.Task) (string, error) {
	var numbers []float64
	if err := json.Unmarshal([]byte(t.Payload), &numbers); err != nil {
		return "", fmt.Errorf("invalid payload: expected JSON array of numbers")
	}

	var sum float64
	for _, n := range numbers {
		sum += n
	}

	time.Sleep(300 * time.Millisecond)
	return fmt.Sprintf("%.2f", sum), nil
}

func Slow(ctx context.Context, t *task.Task) (string, error) {
	select {
	case <-time.After(5 * time.Second):
		return "completed after 5 seconds", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func Flaky(ctx context.Context, t *task.Task) (string, error) {
	time.Sleep(500 * time.Millisecond)

	if rand.Float32() < 0.5 {
		return "", fmt.Errorf("random failure (demo retry)")
	}

	return "succeeded after retry!", nil
}
