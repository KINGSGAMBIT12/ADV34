package main

import (
	"database/sql"
	"fmt"
	"github.com/juju/ratelimit"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

// Car struct represents a car
type Car struct {
	Model    string
	Quantity int
	Price    float64 // Assuming the price is a numeric type, adjust as needed
}

var submitOrderLimiter = ratelimit.NewBucket(time.Second*30, 1)
var (
	orderTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 20px;
        }
    </style>
</head>
<body>
    <h2>{{.Title}}</h2>

    {{range .Cars}}
    <p>Car Model: {{.Model}}</p>
    <p>Quantity: {{.Quantity}}</p>
    <p>Price: ${{.Price}}</p>
    {{end}}
</body>

</html>
`
)

// Database connection string
const connStr = "user=postgres password=12345 dbname=postgres sslmode=disable"

func main() {
	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		fmt.Println("Error opening database:", err)
		return
	}
	defer db.Close()

	// Check database connection
	err = db.Ping()
	if err != nil {
		fmt.Println("Error connecting to the database:", err)
		return
	}

	// Define qty and defaultPrice outside the submitOrderHandler function
	var qty int
	var defaultPrice float64

	r := mux.NewRouter()

	r.HandleFunc("/", indexHandler).Methods("GET")
	r.HandleFunc("/submit_order", submitOrderHandler(db, &qty, &defaultPrice)).Methods("POST")
	r.HandleFunc("/car_info/{carModel}", carInfoHandler(db)).Methods("GET")
	r.HandleFunc("/filter_cars", filterCarsHandler(db)).Methods("POST")
	r.HandleFunc("/sort_cars", sortCarsHandler(db)).Methods("GET")

	http.Handle("/", r)

	fmt.Println("Server is running on :8080")
	http.ListenAndServe(":8080", nil)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("index.html")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tmpl.Execute(w, nil)
}

func submitOrderHandler(db *sql.DB, qty *int, defaultPrice *float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if the rate limit is reached
		if submitOrderLimiter.TakeAvailable(1) < 1 {
			// If the rate limit is reached, return a rate limiting error
			http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
			return
		}

		r.ParseForm()

		carModel := r.Form.Get("carModel")
		quantity := r.Form.Get("quantity")

		// Convert quantity to int
		var err error
		*qty, err = strconv.Atoi(quantity)
		if err != nil {
			fmt.Println("Error converting quantity to int:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Initialize or retrieve defaultPrice based on your application's logic
		*defaultPrice = 0.0

		// Insert data into the database
		_, err = db.Exec("INSERT INTO Cars (CarName, Quantity, Volume, Price) VALUES ($1, $2, $3, $4)", carModel, *qty, 0, *defaultPrice)
		if err != nil {
			fmt.Println("Error inserting data into the database:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Redirect to the /car_info route with the chosen car model
		http.Redirect(w, r, "/car_info/"+carModel, http.StatusSeeOther)
	}
}

func carInfoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		carModel := vars["carModel"]

		// Call a function to retrieve car information from the database based on carModel
		car, err := getCarInfoFromDatabase(db, carModel)
		if err != nil {
			fmt.Println("Error retrieving car information:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Render the car information using the new template (orderTemplate)
		renderTemplate(w, orderTemplate, "Car Information", car)
	}
}

func renderTemplate(w http.ResponseWriter, templateContent, title string, data interface{}) {
	tmpl, err := template.New("template").Parse(templateContent)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var dataSlice []Car
	switch d := data.(type) {
	case Car:
		dataSlice = append(dataSlice, d)
	case []Car:
		dataSlice = d
	default:
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tmpl.Execute(w, map[string]interface{}{
		"Title": title,
		"Cars":  dataSlice,
	})
}

func getCarInfoFromDatabase(db *sql.DB, carModel string) (Car, error) {
	var car Car

	// Query the database to retrieve car information based on carModel
	row := db.QueryRow("SELECT CarName, Quantity, Price FROM Cars WHERE CarName = $1", carModel)
	err := row.Scan(&car.Model, &car.Quantity, &car.Price)
	if err != nil {
		fmt.Println("Error scanning row:", err)
		return Car{}, err
	}

	return car, nil
}

func filterCarsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		// Get the inputted car volume from the form
		carVolume := r.Form.Get("carVolume")

		// Convert car volume to float64
		volume, err := strconv.ParseFloat(carVolume, 64)
		if err != nil {
			fmt.Println("Error converting car volume to float64:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Call a function to filter cars based on volume from the database
		cars, err := filterCarsByVolumeFromDatabase(db, volume)
		if err != nil {
			fmt.Println("Error filtering cars by volume:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Render the filtered cars using the existing template (orderTemplate)
		renderTemplate(w, orderTemplate, "Filtered Cars", cars)
	}
}

func filterCarsByVolumeFromDatabase(db *sql.DB, volume float64) ([]Car, error) {
	var cars []Car

	// Query the database to retrieve cars based on volume
	rows, err := db.Query("SELECT CarName, Quantity, Price FROM Cars WHERE Volume = $1", volume)
	if err != nil {
		fmt.Println("Error querying database:", err)
		return nil, err
	}
	defer rows.Close()

	// Iterate through the result set and populate the cars slice
	for rows.Next() {
		var car Car
		err := rows.Scan(&car.Model, &car.Quantity, &car.Price)
		if err != nil {
			fmt.Println("Error scanning row:", err)
			return nil, err
		}
		cars = append(cars, car)
	}

	return cars, nil
}

func sortCarsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		// Get the sorting order from the form
		sortOrder := r.Form.Get("sort")

		// Call a function to retrieve all cars from the database and sort them
		cars, err := getAllCarsFromDatabase(db, sortOrder)
		if err != nil {
			fmt.Println("Error retrieving and sorting cars:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Render the sorted cars using the existing template (orderTemplate)
		renderTemplate(w, orderTemplate, "Sorted Cars", cars)
	}
}

func getAllCarsFromDatabase(db *sql.DB, sortOrder string) ([]Car, error) {
	var cars []Car

	// Build the SQL query to retrieve all cars and apply sorting
	query := "SELECT CarName, Quantity, Price FROM Cars"
	if sortOrder == "asc" {
		query += " ORDER BY carid ASC"
	} else if sortOrder == "desc" {
		query += " ORDER BY carid DESC"
	}

	// Query the database
	rows, err := db.Query(query)
	if err != nil {
		fmt.Println("Error querying database:", err)
		return nil, err
	}
	defer rows.Close()

	// Iterate through the result set and populate the cars slice
	for rows.Next() {
		var car Car
		err := rows.Scan(&car.Model, &car.Quantity, &car.Price)
		if err != nil {
			fmt.Println("Error scanning row:", err)
			return nil, err
		}
		cars = append(cars, car)
	}

	return cars, nil
}
