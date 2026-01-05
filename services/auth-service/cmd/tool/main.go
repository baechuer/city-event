package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func main() {
	secret := []byte("my-seceret1223")
	tokensFile := "tests/load/tokens.csv"

	f, err := os.Create(tokensFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	var firstToken string

	fmt.Println("Generating 1000 tokens...")
	for i := 0; i < 1000; i++ {
		uid := uuid.New().String()
		claims := jwt.MapClaims{
			"iss":      "cityevents",
			"sub":      uid,
			"uid":      uid,
			"username": fmt.Sprintf("user-%d", i),
			"role":     "user",
			"exp":      time.Now().Add(time.Hour).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		s, err := token.SignedString(secret)
		if err != nil {
			panic(err)
		}
		if i == 0 {
			firstToken = s
		}
		f.WriteString(s + "\n")
	}
	fmt.Println("Done generating tokens.")

	// Create Event
	fmt.Println("Creating Load Test Event...")
	url := "http://localhost:8095/event/v1/events"
	// Use 8092 (Event Service Direct) or 8088 (BFF).
	// Trying Event Service direct.

	requestBody, _ := json.Marshal(map[string]interface{}{
		"title":       "Load Test Event Auto",
		"description": "Auto created by tool",
		"city":        "Sydney",
		"category":    "Tech",
		"start_time":  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"end_time":    time.Now().Add(26 * time.Hour).Format(time.RFC3339),
		"capacity":    100,
	})

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+firstToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error creating event: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Read Body
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	var eventID string
	if data, ok := result["data"].(map[string]interface{}); ok {
		if id, ok := data["id"].(string); ok {
			eventID = id
		}
	} else if id, ok := result["id"].(string); ok {
		eventID = id
	}

	if eventID != "" {
		fmt.Printf("EVENT_ID:%s\n", eventID)
		fmt.Printf("Publishing Event %s...\n", eventID)
		pubURL := fmt.Sprintf("http://localhost:8095/event/v1/events/%s/publish", eventID)
		pubReq, _ := http.NewRequest("POST", pubURL, nil)
		pubReq.Header.Set("Authorization", "Bearer "+firstToken)
		pubResp, err := client.Do(pubReq)
		if err != nil {
			fmt.Printf("Error publishing event: %v\n", err)
		} else {
			defer pubResp.Body.Close()
			if pubResp.StatusCode == 200 {
				fmt.Println("Event Published Successfully")
			} else {
				fmt.Printf("Failed to publish event: %s\n", pubResp.Status)
			}
		}
	} else {
		fmt.Println("Failed to extract ID from response")
	}
}
