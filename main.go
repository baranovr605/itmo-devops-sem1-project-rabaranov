package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type userResponse struct {
	TotalItems      int     `json:"total_items"`
	TotalCategories int     `json:"total_categories"`
	TotalPrice      float64 `json:"total_price"`
}

var (
	hostPsql     string = os.Getenv("PSQL_HOST")
	portPsql     string = os.Getenv("PSQL_PORT")
	userPsql     string = os.Getenv("PSQL_USER")
	passwordPsql string = os.Getenv("PSQL_PASSWORD")
	dbnameDbPsql string = os.Getenv("PSQL_DB_NAME")
)

var postgresDb *sql.DB

func setupPostgres() (*sql.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s "+
		"password=%s dbname=%s sslmode=disable",
		hostPsql, portPsql, userPsql, passwordPsql, dbnameDbPsql)

	postgresDb, err := sql.Open("postgres", psqlInfo)

	if err != nil {
		return nil, err
	}

	err = postgresDb.Ping()
	if err != nil {
		return nil, err
	}

	fmt.Println("Successfully connected to database!")
	return postgresDb, nil
}

// POST for add data in database
func postZipRequest(w http.ResponseWriter, r *http.Request) {
	// Apply zip file
	zipFile, fileHeader, err := r.FormFile("file")

	if err != nil {
		http.Error(w, "Cant read file", http.StatusBadRequest)
		fmt.Println("Error read file with request")
		return
	}
	defer zipFile.Close()

	archive, err := zip.OpenReader(fileHeader.Filename)
	if err != nil {
		panic(err)
	}
	defer archive.Close()

	// Read the file content into a byte buffer
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, zipFile); err != nil {
		http.Error(w, "Error reading file content", http.StatusInternalServerError)
		return
	}

	zipReader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		http.Error(w, "Unable to read zip content", http.StatusInternalServerError)
		return
	}

	// Create transaction
	postgresTx, err := postgresDb.Begin()
	if err != nil {
		http.Error(w, "Cant create transaction", http.StatusInternalServerError)
		return
	}
	// Defer a rollback in case anything fails.
	defer func() {
		if err != nil {
			postgresTx.Rollback()
			return
		}
		err = postgresTx.Commit()
	}()

	var itemCount = 0

	// Read all the files from zip archive
	for _, zipFile := range zipReader.File {
		if strings.HasSuffix(zipFile.Name, ".csv") {
			zipFileOpened, err := zipFile.Open()
			if err != nil {
				http.Error(w, "Cant unzip archive", http.StatusInternalServerError)
				return
			}
			defer zipFileOpened.Close()

			csvReader := csv.NewReader(zipFileOpened)
			csvReader.FieldsPerRecord = 5

			_, err = csvReader.Read()
			if err != nil {
				http.Error(w, "Cant read first line in data.csv", http.StatusInternalServerError)
			}

			for {
				record, err := csvReader.Read()

				// If end of file, then finish
				if err == io.EOF {
					break
				}

				if err != nil {
					http.Error(w, "Cant read data from data.csv", http.StatusInternalServerError)
					break
				}

				// createDate to DATE
				formatedDate, err := time.Parse("2006-01-02", record[4])
				if err != nil {
					http.Error(w, "Cant format date", http.StatusBadRequest)
					return
				}

				_, err = postgresTx.Exec(`INSERT INTO prices (name, category, price, create_date) VALUES ($1, $2, $3, $4)`,
					record[1], record[2], record[3], formatedDate)
				itemCount++
				if err != nil {
					http.Error(w, "Error when write data to database", http.StatusInternalServerError)
					return
				}

			}
		} else {
			http.Error(w, "Cant fint data.csv file", http.StatusInternalServerError)
			return
		}
	}

	var totalCategories int
	var totalPrice float64

	// Select categories and price
	rowSelect := postgresTx.QueryRow(`
    SELECT 
        COUNT(DISTINCT category) AS total_categories,
        COALESCE(SUM(CAST(price AS numeric)), 0) AS total_price
    FROM prices;
	`)

	if err := rowSelect.Scan(&totalCategories, &totalPrice); err != nil {
		http.Error(w, "Cant get total categories and price", http.StatusInternalServerError)
		return
	}

	userResponseDb := userResponse{
		TotalItems:      itemCount,
		TotalCategories: totalCategories,
		TotalPrice:      totalPrice,
	}

	// Encode the userResponse struct to JSON and write to the response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(userResponseDb); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func getZipRequest(w http.ResponseWriter, r *http.Request) {
	// Query for get all data from database
	allRows, err := postgresDb.Query("SELECT id, name, category, price, TO_CHAR(create_date, 'YYYY-MM-DD') FROM prices")
	if err != nil {
		http.Error(w, "Cant read data from database", http.StatusInternalServerError)
		return
	}
	defer allRows.Close()

	// Create data.csv file
	csvFile, err := os.Create("data.csv")
	if err != nil {
		http.Error(w, "Cant create data.csv file", http.StatusInternalServerError)
		return
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)

	// Add header
	headers := []string{"id", "name", "category", "price", "create_date"}
	if err := writer.Write(headers); err != nil {
		http.Error(w, "Cant add header", http.StatusInternalServerError)
		return
	}

	// Write data to data.csv
	for allRows.Next() {
		var id int
		var name string
		var category string
		var price string
		var createDate string
		if err := allRows.Scan(&id, &name, &category, &price, &createDate); err != nil {
			http.Error(w, "Cant scan row data", http.StatusInternalServerError)
			return
		}
		record := []string{strconv.Itoa(id), name, category, price, createDate}
		if err := writer.Write(record); err != nil {
			http.Error(w, "Cant write data to data.csv", http.StatusInternalServerError)
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		http.Error(w, "Error flushing data to data.csv", http.StatusInternalServerError)
		fmt.Println("Error flushing data!")
		return
	}

	// Check errors after write
	if err := allRows.Err(); err != nil {
		http.Error(w, "Error after write data", http.StatusInternalServerError)
		return
	}

	// Create data.zip file
	zipFile, err := os.Create("data.zip")
	if err != nil {
		http.Error(w, "Error when create data.zip", http.StatusInternalServerError)
		return
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)

	// Add data.csv to zip archive
	zipFileWriter, err := zipWriter.Create("data.csv")
	if err != nil {
		http.Error(w, "Cant add data.csv to zip archive", http.StatusInternalServerError)
		return
	}

	// Open data.csv for reading (with data)
	csvFileRead, err := os.Open("data.csv")
	if err != nil {
		http.Error(w, "Cant open data.csv with data to read", http.StatusInternalServerError)
		return
	}
	defer csvFile.Close()

	// Copy data.csv (with data) to data.csv inside data.zip
	if _, err := io.Copy(zipFileWriter, csvFileRead); err != nil {
		http.Error(w, "Cant copy data.csv (with data) to data.csv inside data.zip", http.StatusInternalServerError)
		return
	}

	zipWriter.Close()
	// Send data.zip to user
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="data.zip"`)
	http.ServeFile(w, r, zipFile.Name())
}

func handlerRequests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Handle POST request
		getZipRequest(w, r)
	case http.MethodPost:
		// Handle GET request
		postZipRequest(w, r)
	default:
		// Method not allowed
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func main() {
	var err error
	postgresDb, err = setupPostgres()
	if err != nil {
		panic(err)
	}
	defer postgresDb.Close()

	http.HandleFunc("/api/v0/prices", handlerRequests)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
