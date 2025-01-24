package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type aapconfig struct {
	url            string
	token          string
	instanceGroups []string
}
type Response struct {
	instanceGroup string
	count         float64
}

func sendRequest(urlStr string, token string) (io.ReadCloser, error) {
	client := &http.Client{
		Transport: &http.Transport{},
		Timeout:   30 * time.Second, // Set a timeout of 30 seconds
	}

	var resp *http.Response
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest("GET", urlStr, nil)
		if err != nil {
			fmt.Printf("Error creating request: %v", err)
			return nil, fmt.Errorf("error creating request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token)

		resp, err = client.Do(req)
		if err != nil {
			fmt.Printf("Error sending request: %v", err)
			return nil, fmt.Errorf("error sending request: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			return resp.Body, nil
		}

		if resp.StatusCode == http.StatusGatewayTimeout {
			fmt.Printf("Gateway timeout. Retrying (%d/%d)...", i+1, maxRetries)
			time.Sleep(2 * time.Second) // Wait before retrying
		} else {
			resp.Body.Close()
			return nil, fmt.Errorf("request failed with status: %s", resp.Status)
		}
	}

	return nil, fmt.Errorf("max retries exceeded")
}

func GetCount(aapConfig aapconfig, URL string) float64 {

	body, err := sendRequest(aapConfig.url+URL, aapConfig.token)
	if err != nil {
		fmt.Println(err)
	}
	defer body.Close()

	var initialData map[string]interface{}
	if err := json.NewDecoder(body).Decode(&initialData); err != nil {
		fmt.Println(err)
	}
	count := initialData["count"].(float64)
	return count
}

func fetchPendingJobsPeriodically(aapConfig aapconfig, ch chan<- []Response) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			responses := []Response{}
			for _, instanceGroup := range aapConfig.instanceGroups {
				responses = append(responses, Response{instanceGroup, GetCount(aapConfig, fmt.Sprintf("/api/v2/jobs/?instance_group__name__icontains=%s&or__status=pending", instanceGroup))})
			}
			ch <- responses
		}
	}
}

func printJob(pending []Response) {
	for _, response := range pending {
		fmt.Printf("InstanceGroup Name: %s\n", response.instanceGroup)
		fmt.Printf("Count: %f\n", response.count)
	}

}
