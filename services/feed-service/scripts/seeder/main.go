package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Seed data generator for feed-service development
// Only runs when ENV=dev AND ALLOW_SEED=1

func main() {
	if os.Getenv("ENV") != "dev" || os.Getenv("ALLOW_SEED") != "1" {
		log.Fatal("Seed only allowed in dev with ALLOW_SEED=1")
	}

	dbURL := os.Getenv("DB_ADDR")
	if dbURL == "" {
		dbURL = "postgres://user:pass@localhost:5432/feed_db?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer pool.Close()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Generate events
	events := generateEvents(rng, 100)
	if err := insertEvents(ctx, pool, events); err != nil {
		log.Fatalf("failed to insert events: %v", err)
	}
	log.Printf("Inserted %d events", len(events))

	// Generate users
	users := generateUsers(rng, 50)
	if err := insertUsers(ctx, pool, users); err != nil {
		log.Fatalf("failed to insert users: %v", err)
	}
	log.Printf("Inserted %d users", len(users))

	// Generate joins with Zipf distribution (some events are popular)
	joins := generateJoins(rng, users, events, 500)
	if err := insertJoins(ctx, pool, joins); err != nil {
		log.Fatalf("failed to insert joins: %v", err)
	}
	log.Printf("Inserted %d joins", len(joins))

	// Generate views
	views := generateViews(rng, users, events, 2000)
	if err := insertViews(ctx, pool, views); err != nil {
		log.Fatalf("failed to insert views: %v", err)
	}
	log.Printf("Inserted %d views", len(views))

	log.Println("Seed complete!")
}

type Event struct {
	ID        string
	Title     string
	City      string
	Tags      []string
	StartTime time.Time
}

type User struct {
	ID       string
	ActorKey string
}

type UserEvent struct {
	ActorKey   string
	EventType  string
	EventID    string
	BucketDate time.Time
	OccurredAt time.Time
}

var cities = []string{"Sydney", "Melbourne", "Brisbane", "Perth", "Adelaide"}
var tags = []string{"music", "tech", "food", "sports", "art", "networking", "outdoors", "gaming", "wellness", "education"}
var titlePrefixes = []string{"Annual", "Weekly", "Monthly", "Community", "Downtown", "Weekend", "Evening"}
var titleSuffixes = []string{"Meetup", "Festival", "Conference", "Workshop", "Social", "Gathering", "Party"}

func generateEvents(rng *rand.Rand, count int) []Event {
	events := make([]Event, count)
	now := time.Now()

	for i := 0; i < count; i++ {
		// Start time: random day in next 30 days
		startTime := now.Add(time.Duration(rng.Intn(30*24)) * time.Hour)

		// Random tags (1-3)
		numTags := 1 + rng.Intn(3)
		eventTags := make([]string, numTags)
		for j := 0; j < numTags; j++ {
			eventTags[j] = tags[rng.Intn(len(tags))]
		}

		events[i] = Event{
			ID:        uuid.NewString(),
			Title:     fmt.Sprintf("%s %s %s", titlePrefixes[rng.Intn(len(titlePrefixes))], eventTags[0], titleSuffixes[rng.Intn(len(titleSuffixes))]),
			City:      cities[rng.Intn(len(cities))],
			Tags:      eventTags,
			StartTime: startTime,
		}
	}
	return events
}

func generateUsers(rng *rand.Rand, count int) []User {
	users := make([]User, count)
	for i := 0; i < count; i++ {
		id := uuid.NewString()
		users[i] = User{
			ID:       id,
			ActorKey: "u:" + id,
		}
	}
	return users
}

// generateJoins uses Zipf distribution - some events get many more joins
func generateJoins(rng *rand.Rand, users []User, events []Event, count int) []UserEvent {
	joins := make([]UserEvent, 0, count)
	seen := make(map[string]bool)

	// Zipf: rank^(-s) probability, s=1.5
	s := 1.5
	weights := make([]float64, len(events))
	total := 0.0
	for i := range events {
		w := math.Pow(float64(i+1), -s)
		weights[i] = w
		total += w
	}

	for len(joins) < count {
		// Pick user uniformly
		user := users[rng.Intn(len(users))]

		// Pick event by Zipf weight
		r := rng.Float64() * total
		cumulative := 0.0
		eventIdx := 0
		for i, w := range weights {
			cumulative += w
			if r <= cumulative {
				eventIdx = i
				break
			}
		}
		event := events[eventIdx]

		// Check for duplicate (same user-event-day)
		// Only go back 2 days to stay within January 2026 partition
		bucketDate := time.Now().Add(-time.Duration(rng.Intn(2*24)) * time.Hour).Truncate(24 * time.Hour)
		key := fmt.Sprintf("%s:%s:%s", user.ActorKey, event.ID, bucketDate.Format("2006-01-02"))
		if seen[key] {
			continue
		}
		seen[key] = true

		joins = append(joins, UserEvent{
			ActorKey:   user.ActorKey,
			EventType:  "join",
			EventID:    event.ID,
			BucketDate: bucketDate,
			OccurredAt: bucketDate.Add(time.Duration(rng.Intn(24)) * time.Hour),
		})
	}
	return joins
}

func generateViews(rng *rand.Rand, users []User, events []Event, count int) []UserEvent {
	views := make([]UserEvent, 0, count)
	seen := make(map[string]bool)

	for len(views) < count {
		user := users[rng.Intn(len(users))]
		event := events[rng.Intn(len(events))]
		// Only go back 2 days to stay within January 2026 partition
		bucketDate := time.Now().Add(-time.Duration(rng.Intn(2*24)) * time.Hour).Truncate(24 * time.Hour)

		key := fmt.Sprintf("%s:%s:view:%s", user.ActorKey, event.ID, bucketDate.Format("2006-01-02"))
		if seen[key] {
			continue
		}
		seen[key] = true

		views = append(views, UserEvent{
			ActorKey:   user.ActorKey,
			EventType:  "view",
			EventID:    event.ID,
			BucketDate: bucketDate,
			OccurredAt: bucketDate.Add(time.Duration(rng.Intn(24)) * time.Hour),
		})
	}
	return views
}

func insertEvents(ctx context.Context, pool *pgxpool.Pool, events []Event) error {
	for _, e := range events {
		_, err := pool.Exec(ctx, `
			INSERT INTO event_index (event_id, title, city, tags, start_time, status, synced_at)
			VALUES ($1, $2, $3, $4, $5, 'published', NOW())
			ON CONFLICT (event_id) DO NOTHING
		`, e.ID, e.Title, e.City, e.Tags, e.StartTime)
		if err != nil {
			return err
		}
	}
	return nil
}

func insertUsers(ctx context.Context, pool *pgxpool.Pool, users []User) error {
	// Users are implicit in user_events through actor_key
	// No separate user table in feed-service
	return nil
}

func insertJoins(ctx context.Context, pool *pgxpool.Pool, joins []UserEvent) error {
	for _, j := range joins {
		_, err := pool.Exec(ctx, `
			INSERT INTO user_events (actor_key, event_type, event_id, bucket_date, occurred_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (actor_key, event_id, event_type, bucket_date) DO NOTHING
		`, j.ActorKey, j.EventType, j.EventID, j.BucketDate, j.OccurredAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func insertViews(ctx context.Context, pool *pgxpool.Pool, views []UserEvent) error {
	for _, v := range views {
		_, err := pool.Exec(ctx, `
			INSERT INTO user_events (actor_key, event_type, event_id, bucket_date, occurred_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (actor_key, event_id, event_type, bucket_date) DO NOTHING
		`, v.ActorKey, v.EventType, v.EventID, v.BucketDate, v.OccurredAt)
		if err != nil {
			return err
		}
	}
	return nil
}
