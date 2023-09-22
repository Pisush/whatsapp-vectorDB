package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/pisush/fin-chat/embed"
	"github.com/pisush/fin-chat/upsert"
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
	topK           = 1        // how many results do we want back

	embeddingModel = "text-embedding-ada-002"
	// format example: [09.09.23, 14:35:02] ~ john_doe: Hello world!
	enFileToEmbedPath = "./en_files/en_chat.txt"
	heFileToEmbedPath = "./he_files/he_chat.txt"
	//format example: "Hello world!",0.12345,0.67890,0.11121,...,0.56433
	enEmbeddedCSVPath = "./en_files/en_embeddings.csv"
	heEmbeddedCSVPath = "./he_files/he_embeddings.csv"
)

// Used to parse the response from a query to the Pinecone index.
type QueryResponse struct {
	ID           string    `json:"id"`
	Score        float64   `json:"score"`
	Values       []float64 `json:"values"`
	SparseValues struct {
		Indices []int     `json:"indices"`
		Values  []float64 `json:"values"`
	} `json:"sparseValues"`
	Metadata map[string]interface{} `json:"metadata"`
}

type QueryResponseBody struct {
	Matches   []QueryResponse `json:"matches"`
	Namespace string          `json:"namespace"`
}

func getPcProjectID(log *log.Logger) (string, error) {
	whoamiURL := pcCtrlPrefix + pcEnv + pcAPIURL + pcProjectIDPath
	req, err := http.NewRequest(http.MethodGet, whoamiURL, nil)
	if err != nil {
		log.Printf("Error creating new request: %v", err)
		return "", err
	}
	req.Header.Set("Api-Key", pcAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error in HTTP request: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding response: %v", err)
		return "", err // Ensure we return here
	}

	pcProjectID, ok := result["project_name"].(string)
	if !ok {
		return "", fmt.Errorf("project_name not found or is not a string")
	}

	return pcProjectID, nil
}

// Helper func: Input is a string, and output are the nearest strings
func queryPinecone(indexName, queryMessage, pcProjectID string, log *log.Logger) ([]QueryResponse, error) {

	// Prepare query
	url := "https://" + indexName + "-" + pcProjectID + ".svc." + pcEnv + pcAPIURL + "query"

	// Embed the query message to get the query vector
	queryVector, err := embed.GetEmbedding(queryMessage, embeddingModel)
	if err != nil {
		log.Printf("Error embedding query message: %v", err)
		return nil, fmt.Errorf("error embedding query message: %v", err)
	}

	queryData := map[string]interface{}{
		"includeValues":   "false",
		"includeMetadata": "false",
		"topK":            topK,
		"vector":          queryVector,
	}

	jsonData, err := json.Marshal(queryData)
	if err != nil {
		fmt.Println("Error marshalling query data: ", err)
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating new request: ", err)
		return nil, err
	}

	req.Header.Set("accept", "application/json")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("Api-Key", pcAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		log.Printf("Error sending request: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	var response QueryResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Error decoding response body: %v", err)
		return nil, err
	}

	matches := response.Matches

	// Fetch vector content for each match
	for _, match := range matches {
		fetchURL := fmt.Sprintf("https://%s-%s.svc.%s.pinecone.io/vectors/fetch?ids=%s", indexName, pcProjectID, pcEnv, match.ID)

		fetchReq, err := http.NewRequest("GET", fetchURL, nil)
		if err != nil {
			log.Printf("Error creating new request to fetch vector: %v", err)
			return nil, err
		}
		fetchReq.Header.Set("Api-Key", pcAPIKey)
		fetchReq.Header.Set("Accept", "application/json")

		fetchResp, err := client.Do(fetchReq)
		if err != nil {
			log.Printf("Error in HTTP request to fetch vector: %v", err)
			return nil, err
		}
		defer fetchResp.Body.Close()

		var fetchResponse struct {
			Vectors map[string]struct {
				ID     string    `json:"id"`
				Values []float64 `json:"values"`
			} `json:"vectors"`
			Namespace string `json:"namespace"`
		}

		if err := json.NewDecoder(fetchResp.Body).Decode(&fetchResponse); err != nil {
			log.Printf("Error decoding fetch response: %v", err)
			return nil, err
		}

		if vectorData, exists := fetchResponse.Vectors[match.ID]; exists {
			match.Values = vectorData.Values
			log.Printf("Fetched vector content for ID %s: %v", vectorData.ID, vectorData.Values)
		} else {
			log.Printf("No vector content found for ID %s", match.ID)
		}

	}

	return matches, nil

}

func promptUserAndQueryPinecone(indexName, pcProjectID string, log *log.Logger) error {
	reader := bufio.NewReader(os.Stdin)
	client := &http.Client{}

	for {
		// Ask the user to provide a query
		fmt.Print("Please enter a message to search for (or type 'end' to exit): ")
		queryMessage, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error reading user input: %v", err)
			return err
		}

		// Trim the newline character from the input
		queryMessage = strings.TrimSpace(queryMessage)

		// Check if the user entered "end", and if so, exit the loop
		if strings.ToLower(queryMessage) == "end" {
			fmt.Println("You typed exit. Program exiting!")
			break
		}

		// Call queryPinecone with the queryMessage
		queryResponse, err := queryPinecone(indexName, queryMessage, pcProjectID, log)
		if err != nil {
			log.Printf("Error querying Pinecone: %v", err)
			continue
		}

		// Get message based on vector ID
		for _, match := range queryResponse {
			fetchURL := "https://" + indexName + "-" + pcProjectID + ".svc." + pcEnv + pcAPIURL + "vectors/fetch?ids=" + match.ID
			fetchReq, err := http.NewRequest("GET", fetchURL, nil)
			if err != nil {
				log.Printf("Error creating fetch request: %v", err)
				return err
			}
			fetchReq.Header.Set("Api-Key", pcAPIKey)
			fetchReq.Header.Set("accept", "application/json")

			log.Printf("Attempting to fetch vector content for ID %s", match.ID)

			fetchResp, err := client.Do(fetchReq)
			if err != nil {
				log.Printf("Error sending fetch request: %v", err)
				return err
			}
			defer fetchResp.Body.Close()

			fmt.Println(">>fetchResp")
			fmt.Println(fetchResp)

			var fetchResponse struct {
				Vectors map[string]struct {
					ID     string    `json:"id"`
					Values []float64 `json:"values"`
				} `json:"vectors"`
				Namespace string `json:"namespace"`
			}

			if err := json.NewDecoder(fetchResp.Body).Decode(&fetchResponse); err != nil {
				fmt.Println("Error decoding fetch response", fetchResp)
				log.Printf("Error decoding fetch response: %v", err)
				return err
			}

			if vectorData, exists := fetchResponse.Vectors[match.ID]; exists {
				match.Values = vectorData.Values
				fmt.Println("Fetched vector content for ID", vectorData.ID)
				fmt.Println(vectorData.Values)

				log.Printf("Fetched vector content for ID %s: %v", vectorData.ID, vectorData.Values)
			} else {
				log.Printf("No vector content found for ID %s", match.ID)
				fmt.Println("no vector content for ID", vectorData.ID)
			}
		}
	}

	return nil
}

func main() {
	// Setup logs
	logFile, err := os.OpenFile("err.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("opening err log file: %v", err)
	}
	defer logFile.Close()

	log := log.New(logFile, "ERR: ", log.Ldate|log.Ltime)

	// Get user action
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("What is the action? Options are: embed/upsert/query")
	action, _ := reader.ReadString('\n')
	action = strings.TrimSpace(action)
	actions := strings.Fields(action)

	if len(actions) == 0 {
		fmt.Println("No action specified.")
		return
	}

	inputFileName := enFileToEmbedPath
	embeddingsFileName := enEmbeddedCSVPath

	fmt.Print("Choose language (en/he): ")
	lang, _ := reader.ReadString('\n')
	lang = strings.TrimSpace(lang)
	if lang == "he" {
		inputFileName = heFileToEmbedPath
		embeddingsFileName = heEmbeddedCSVPath
	} else {
		fmt.Println("Unknown language. Please specify 'en' or 'he'.")
		return
	}

	// Execute the user request
	for _, act := range actions {
		switch act {
		case "embed":

			err = embed.CreateEmbeddingFile(inputFileName, embeddingsFileName, embeddingModel, log)
			if err != nil {
				log.Fatalf("Error creating embedding file: %v", err)
				fmt.Println("Error embedding", err)
				return
			}

		case "upsert":
			if inputFileName == "" || embeddingsFileName == "" {
				fmt.Println("Embedding must be done before upserting.")
				return
			}
			// Ensure Pinecone index exists
			err = upsert.GetOrCreatePineconeIndex(indexName, log)
			if err != nil {
				log.Fatalf("Error ensuring Pinecone index exists: %v", err)
			}

			// Upsert data to Pinecone
			err = upsert.UpsertDataToPinecone(indexName, embeddingsFileName, log)
			if err != nil {
				fmt.Println("Failed upserting data to pinecone", err)
				log.Printf("Error upserting data to Pinecone: %v", err)
				return
			}

		case "query":
			pcProjectID, _ := getPcProjectID(log)
			// Call the function to prompt the user and query Pinecone
			err = promptUserAndQueryPinecone(indexName, pcProjectID, log)
			if err != nil {
				fmt.Println("Error in the query proces: ", err)
				fmt.Println("There was an Error in the query proces: ")
				log.Fatalf("Error in the query process: %v", err)
			}

		default:
			fmt.Println("Unknown action: ", act)
			return
		}

		// Wrapping up before closing
		if err := logFile.Sync(); err != nil {
			log.Fatalf("Failed to flush err log file: %v", err)
		}
	}
}
