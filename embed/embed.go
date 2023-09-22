package embed

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	openAIAPIKey   = "Bearer sk-xxx"
	embeddingModel = "text-embedding-ada-002"
	embeddingsURL  = "https://api.openai.com/v1/embeddings"
)

type ResponseData struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// Obtains an embedding for a given line
func GetEmbedding(text string, model string) ([]float64, error) {
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "'", "'\\''")

	body := fmt.Sprintf(`{"input": ["%s"], "model": "%s"}`, text, model)
	req, err := http.NewRequest("POST", embeddingsURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", openAIAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request error: %w", err)
	}
	defer resp.Body.Close()

	var responseData ResponseData

	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return nil, err
	}

	if len(responseData.Data) == 0 || len(responseData.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("no data in response")
	}

	return responseData.Data[0].Embedding, nil
}

// Creates a csv file in the format: (embedding []float64)
func CreateEmbeddingFile(inputFileName string, embeddingsFileName string, embeddingModel string, log *log.Logger) error {
	// Initialize counters
	var linesProcessed, parseFailures, embeddingFailures, writeFailures, successCount int

	// In case embeddings work well and no temp files needed - delete this block
	// get the current date and time to add as a suffix to the file name
	currentTime := time.Now()
	suffix := currentTime.Format("01-02-15-04")
	// append suffix to embeddingsFileName
	embeddingsFileName = fmt.Sprintf("%s-%s", embeddingsFileName, suffix)

	// create embeddings file
	embedFile, err := os.Create(embeddingsFileName)
	if err != nil {
		log.Fatalf("In CreateEmbeddingsFile: Can't open embeddings file: %v", err)
		return err
	}
	defer embedFile.Close()

	csvWriter := csv.NewWriter(embedFile)
	defer csvWriter.Flush()

	// parse input and obtain embeddings
	parsedFile, err := os.Open(inputFileName)
	if err != nil {
		log.Fatalf("In CreatingEmbeddingsFile: Error opening input file: %v", err)
		return err
	}
	defer parsedFile.Close()

	scanner := bufio.NewScanner(parsedFile)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		re := regexp.MustCompile(`(?:\[.*?\]\s*:\s*~?|^)(\S+)`)

		matches := re.FindStringSubmatch(line)
		linesProcessed++ // Increment the lines processed counter

		var message string
		if len(matches) == 3 {
			message = matches[2]
		} else if len(matches) == 2 {
			message = matches[1]
		} else {
			parseFailures++ // Increment the parse failures counter
			log.Printf("Unable to parse line %d of length %d - skipping: Content: %s\n", lineNumber, len(matches), line)
		}

		embedding, err := GetEmbedding(message, embeddingModel)
		if err != nil {
			embeddingFailures++ // Increment the embedding failures counter
			log.Printf("Error getting embedding for line %d: %s - %v\n", lineNumber, line, err)
			continue
		}

		strEmbedding := float64ToStringSlice(embedding)
		err = csvWriter.Write(strEmbedding)
		if err != nil {
			writeFailures++ // Increment the write failures counter
			log.Printf("Error writing record to CSV at line %d: %v\n", lineNumber, err)
			continue
		}
		successCount++ // Increment the success counter

	}
	log.Printf("Process Summary: Lines Processed=%d, Parse Failures=%d, Embedding Failures=%d, Write Failures=%d, Successes=%d", linesProcessed, parseFailures, embeddingFailures, writeFailures, successCount)
	fmt.Println("Process Summary: Lines Processed =", linesProcessed, ", Parse Failures =", parseFailures, ", Embedding Failures =", embeddingFailures, ", Write Failures =", writeFailures, ", Successes =", successCount)

	if err := scanner.Err(); err != nil {
		log.Fatalf("Scanner error: %v", err)
	}

	return nil
}

// Utility function to convert a slice of float64 to a slice of string
func float64ToStringSlice(floats []float64) []string {
	strs := make([]string, len(floats))
	for i, f := range floats {
		strs[i] = fmt.Sprintf("%f", f)
	}
	return strs
}
