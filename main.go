// Documentation
//
// Secret santa app that's random so people don't have to worry about the picking process.
// No emails or login, just use the domain.
//
// Run it with `go run .`
//
//
// TODOs
//
// TODO: Command line flags
// - serve to serve it
// - unpick to unpick all
// - editor (to have one where i can edit the gob file manually)
//
// TODO: Make it safer by doing this.
// 1. Have no ID
// 2. Choose name
// 3. Get (and store in local storage)
// -  other person's name & list
// -  this person's name & list
// -  token to make requests
// 4. Set this ID picked on server
//
// -> This data is used to know if the person has picked or not.
//
// 1. Already have an ID (in local storage)
// 2. Get
// - Wishlists (for sync)

package main

//- Libraries
import (
	"encoding/gob"
 "encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"math/rand"
	"net/http"
	"os/signal"
	"strings"
 "strconv"

	_ "embed"
	"log"
	"os"
)

// - Types
type Person struct {
	Name      string
	Other     int
	Wishlist  string
	HasPicked bool
	Token     int64
}

type PageData struct {
	People []Person
	KeyID  int64
}

//- Globals

var DEBUG = true
var local_storage_key_id int64

//go:embed index.tmpl.html
var page_html string

//- Globals
var global_people = []Person{
	{Name: "Nawel"},
	{Name: "Tobias"},
	{Name: "Luca"},
	{Name: "Lola"},
	{Name: "Aeris"},
	{Name: "Lionel"},
	{Name: "Aurélie"},
	{Name: "Sean"},
	{Name: "Émilie"},
	{Name: "Yves"},
	{Name: "Marthe"},
}

var global_version int = 1
var data_file_name string = "people.gob"

//- Serializing
func EncodeData(logger *log.Logger, file_name string) {
	file, err := os.Create(file_name)
	if err != nil {
		logger.Fatalln(err)
	}
	defer file.Close()

	enc := gob.NewEncoder(file)
	if err := enc.Encode(global_version); err != nil {
		logger.Fatalln(err)
	}
	if err := enc.Encode(global_people); err != nil {
		logger.Fatalln(err)
	}
}


func DecodeData(logger *log.Logger, file_name string) {
	file, err := os.Open(file_name)
	if errors.Is(err, os.ErrNotExist) {
		logger.Println("Datafile does not exist.  Creating", file_name)
		file, err = os.Create(file_name)
		if err != nil {
			logger.Fatalln(err)
		}
  EncodeData(logger, file_name)

	} else if err != nil {
		logger.Fatalln(err)
	} else {
		dec := gob.NewDecoder(file)

		var version int
		if err := dec.Decode(&version); err != io.EOF && err != nil {
			logger.Fatalln(err)
		}
		if version != global_version {
			logger.Fatalf("Version mismatch for datafile@%d != package@%d\n", version, global_version)
		}

		if err := dec.Decode(&global_people); err != nil && err != io.EOF {
			logger.Fatalln(err)
		}

		logger.Printf("Imported %d people.\n", len(global_people))

		if err := file.Close(); err != nil {
			logger.Fatalln(err)
		}
	}
}

//- Person
func (person Person) String() string {
	var digits string
	if person.Token > 99999 {
		digits = fmt.Sprintf("%d", person.Token)[:6]
	} else {
		digits = fmt.Sprintf("%d", person.Token)
	}

	return fmt.Sprintf("%s_%s(%t)\n%s\n",
		person.Name, digits, person.HasPicked, person.Wishlist)
}

func TemplateToString(page_html string, people []Person, seed int64) string {
	var buf strings.Builder
	template_response, err := template.New("roulette").Parse(page_html)
	if err != nil {
		fmt.Println(err)
	}
	err = template_response.ExecuteTemplate(&buf, "roulette", PageData{people, local_storage_key_id})
	if err != nil {
		fmt.Println(err)
	}
	response := buf.String()

	return response
}

func FindPersonByName(people []Person, name string) (bool, *Person) {
	var found_person *Person
	var found bool

	for index, value := range people {
		if name == value.Name {
			found_person = &people[index]
			found = true
			break
		}
	}

	return found, found_person
}

func FindPersonByOtherName(people []Person, name string) (bool, *Person) {
	var found_person *Person
	var found bool

	for index, person := range people {
		if name == people[person.Other].Name {
			found = true
			found_person = &people[index]
			break
		}
	}

	return found, found_person
}

func ShufflePeople(rand *rand.Rand, people []Person, logger *log.Logger) {
	// Get a shuffled list that has following constaints
	// 1. One person cannot choose itself
	// 2. Every person has picked another person
	var list []int
	correct := false
	for !correct {
		list = rand.Perm(len(people))

		correct = true
		for i, version := range list {
			if version == i {
				logger.Println("incorrect, need to reshuffle")
				correct = false
				break
			}
		}
	}

	// Initialize people
	for index, value := range list {
		people[index].Other = value
	}
}


// - Main
func main() {
	logger := log.New(os.Stdout, "[noel] ", log.Ldate|log.Ltime)

	seed := rand.Int63()
	// seed = 1623876946084255669
	logger.Println("seed:", seed)
	source := rand.NewSource(seed)
	seeded_rand := rand.New(source)
	rand.Seed(seed)

 // Init people
 {
  ShufflePeople(seeded_rand, global_people, logger)

  local_storage_key_id = rand.Int63()
  for index := range global_people {
   global_people[index].Token = seeded_rand.Int63() / 10000
  }

  DecodeData(logger, data_file_name)

  fmt.Println(global_people)
 }

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c

		EncodeData(logger, data_file_name)

		logger.Println("data saved.")
		os.Exit(0)
	}()

	defer EncodeData(logger, data_file_name)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./assets"))))

 // TODO: replace these by command line flags
	// http.HandleFunc("/unpickall/", func(writer http.ResponseWriter, request *http.Request) {
	// 	for index := range global_people {
	// 		global_people[index].HasPicked = false
	// 	}
	// 	fmt.Fprintln(writer, "Done.")
	// })

	// http.HandleFunc("/shuffle/", func(writer http.ResponseWriter, request *http.Request) {
	//  ShufflePeople(seeded_rand, global_people, logger)
	//
	//  fmt.Fprintln(writer, "Done.")
	// })

	// http.HandleFunc("/addlola/", func(writer http.ResponseWriter, request *http.Request) {
	//  global_people = append(global_people, Person{Name:"Lola", Token:seeded_rand.Int63()})
	//
	//  fmt.Fprintln(writer, "Done.")
	// })

	http.HandleFunc("/list/", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			name := request.FormValue("name")
			text := request.FormValue("text")
			token := request.FormValue("token")

			logger.Println("Edit wishlist of", name)
			found, person := FindPersonByName(global_people, name)

			if found {
    tokenString := strconv.FormatInt(person.Token, 10)
    if token == tokenString {
     person.Wishlist = text
     person.HasPicked = true
     logger.Println(global_people)

     fmt.Fprintln(writer, "ok")
    } else {
     http.Error(writer, "invalid token", http.StatusNotFound)
    }
			} else {
    http.Error(writer, "no such person", http.StatusNotFound)
			}
		} else if request.Method == http.MethodGet {
			params := request.URL.Query()
			name := params.Get("name")
   token := params.Get("token")

   found, person := FindPersonByName(global_people, name)

			if found {
    logger.Println(len(token))
    tokenString := strconv.FormatInt(person.Token, 10)

    if token == tokenString {
     fmt.Fprint(writer, global_people[person.Other].Wishlist)
    } else {
     http.Error(writer, "invalid token", http.StatusNotFound)
    }
			} else {
    http.Error(writer, "no such person", http.StatusNotFound)
			}
		}
	})

	http.HandleFunc("/choose/", func(writer http.ResponseWriter, request *http.Request) {
			params := request.URL.Query()
			name := params.Get("name")
   found, person := FindPersonByName(global_people, name)
   if found {
    if !person.HasPicked {
    person.HasPicked = true

    type Response struct {
     Token int64
     ThisWishlist string
     OtherName string
     OtherWishlist string
    }
    other_person := global_people[person.Other]

    response := Response{person.Token, person.Wishlist, other_person.Name, other_person.Wishlist}

    json.NewEncoder(writer).Encode(response)
    } else {
     http.Error(writer, "person already picked", http.StatusNotFound)
    }
   } else {
     http.Error(writer, "no such person", http.StatusNotFound)
   }
 })

	// Execute the template before-hand since the contents won't change.
	response := TemplateToString(page_html, global_people, seed)
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		if DEBUG {
			buffer, err := os.ReadFile("index.tmpl.html")
			if err != nil {
				fmt.Print(err)
			}

			file_contents := string(buffer)
			response = TemplateToString(file_contents, global_people, seed)
		}
		fmt.Fprint(writer, response)
	})

	var address string
	if DEBUG {
		address = "0.0.0.0:15118"
	} else {
		address = "localhost:15118"
	}
	logger.Printf("Listening on http://%s\n", address)
	if err := http.ListenAndServe(address, nil); err != nil {
		panic(err)
	}

	return
}
