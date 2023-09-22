package upsert

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	pcAPIKey                     = "PINECONE-API-Key"
	pcEnv                        = "gcp-starter" // Other envs: https://docs.pinecone.io/docs/projects
	pcAPIURL                     = ".pinecone.io/"
	pcCtrlPrefix                 = "https://controller."
	pcProjectIDPath              = "actions/whoami"
	pcCreateorConnectToIndexPath = "databases/"
	pcVectorUpsert               = "vectors/upsert"

	indexName      = "whatsapp-chat"
	indexDimension = 1536     // stadnard response size from OpenAI's Ada-002
	indexMetric    = "cosine" // or eculidean or dotproduct: https://docs.pinecone.io/docs/indexes#distance-metrics
)

// Used for upserting data to the vector DBs
type UpsertData struct {
	Metadata  map[string]string `json:"metadata"` // TODO: here goes the original message
	ID        string            `json:"id"`
	Values    []float64         `json:"values"`
	Namespace string            `json:"namespace"`
}

func GetOrCreatePineconeIndex(indexName string, log *log.Logger) error {
	// Step 1: Establish a connection to the index
	connectionURL := pcCtrlPrefix + pcEnv + pcAPIURL + pcCreateorConnectToIndexPath + indexName
	req, err := http.NewRequest(http.MethodGet, connectionURL, nil)
	if err != nil {
		log.Printf("Error in getOrCreatePineconeIndex: can't create a new Get request to establish connection: %v", err)
		return err
	}
	req.Header.Set("Api-Key", pcAPIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		log.Printf("Error in getOrCreatePineconeIndex: can't do the POST request to establish connection: %v", err)

		return err
	}
	defer resp.Body.Close()

	// Check the response to see if the index exists
	if resp.StatusCode != http.StatusOK {
		// Step 2: If the index does not exist, create it
		fmt.Println("Index doesn't exist, creating a new one", indexName)
		log.Printf("Index " + indexName + "not found, creating a new one")
		createIndexURL := pcCtrlPrefix + pcEnv + pcAPIURL + pcCreateorConnectToIndexPath
		client := &http.Client{}
		// Creating a structured data to send as JSON
		data := map[string]interface{}{
			"name":      indexName,
			"dimension": indexDimension, // Assuming 'dimension' is a predefined constant with the correct value
			"metric":    indexMetric,    // Assuming 'metric' is a predefined constant with the correct value
		}
		jsonData, err := json.Marshal(data)
		if err != nil {
			log.Printf("Error marshalling data: %v", err)
			return err
		}

		// Create a new request to check if the index exists
		req, err := http.NewRequest(http.MethodPost, createIndexURL, bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Error in getOrCreatePineconeIndex: can't create a new POST request to create index: %v", err)
			return err
		}
		req.Header.Set("Api-Key", pcAPIKey)
		req.Header.Set("Content-Type", "application/json")

		// Send the request and reading the response
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error in getOrCreatePineconeIndex: can't do the POST request to create index: %v", err)
			return err
		}
		defer resp.Body.Close()

		// Handle the response
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Error reading response body: %v", err)
			} else {
				log.Printf("Failed to create index, status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
			}
			return fmt.Errorf("failed to create index, status code: %d", resp.StatusCode)
		}
		fmt.Println("Successfully created index: ", indexName)
		log.Printf("Successfully created index: %s", indexName)
	}

	return nil
}

func UpsertDataToPinecone(indexName string, filePath string, log *log.Logger) error {
	// Step 1: Get the project ID
	fmt.Println("Upserting from: ", filePath)
	whoamiURL := pcCtrlPrefix + pcEnv + pcAPIURL + pcProjectIDPath
	req, err := http.NewRequest(http.MethodGet, whoamiURL, nil)
	if err != nil {
		log.Printf("Error creating new request: %v", err)
		return err
	}
	req.Header.Set("Api-Key", pcAPIKey)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error in HTTP request: %v", err)
		return err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		log.Printf("Error decoding response: %v", err)
		return err
	}
	pcProjectID := result["project_name"].(string)

	// Step 2: Upsert data
	upsertURL := "https://" + indexName + "-" + pcProjectID + ".svc." + pcEnv + pcAPIURL + pcVectorUpsert

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	lineNumber := 0
	successCount := 0
	failCount := 0

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		valuesStr := strings.Split(line, ",")
		values := make([]float64, len(valuesStr))
		for i, v := range valuesStr {
			values[i], err = strconv.ParseFloat(v, 64)
			if err != nil {
				log.Printf("Error parsing float value at line %d: %v", lineNumber, err)
				continue
			}
		}

		data := map[string]interface{}{
			"vectors": []map[string]interface{}{
				{
					"id":     fmt.Sprintf("vector_id_%d", lineNumber),
					"values": values,
				},
			},
		}

		jsonData, err := json.Marshal(data)
		if err != nil {
			log.Printf("Error marshalling data at line %d: %v", lineNumber, err)
			continue
		}

		req, err := http.NewRequest(http.MethodPost, upsertURL, bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Error creating new request at line %d: %v", lineNumber, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Api-Key", pcAPIKey)

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error in HTTP request at line %d: %v", lineNumber, err)
			failCount++
			continue
		}

		if resp.StatusCode >= 400 {
			log.Printf("HTTP error at line %d: %s", lineNumber, resp.Status)
			failCount++
		} else {
			successCount++
		}
		resp.Body.Close()
	}

	log.Printf("Process Summary: Lines Processed=%d, Upserted Successfully=%d, Failed=%d", lineNumber, successCount, failCount)
	fmt.Printf("Process Summary: Lines Processed=%d, Upserted Successfully=%d, Failed=%d\n", lineNumber, successCount, failCount)

	if err := scanner.Err(); err != nil {
		log.Printf("Scanner error: %v", err)
		return err
	}

	return nil
}
