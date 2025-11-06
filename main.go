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
	"flag"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"os/signal"
	"strconv"
	"strings"

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

//- Globals

var nil_person = Person{Name: "nil"}
var global_local_storage_key int64
var global_local_storage_key_initialized = false

//go:embed index.tmpl.html
var page_html string

// - Globals
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
var global_people_initialized = false

var global_version int = 2
var global_data_file_name string = "people.gob"

func EncodeWrapper(encoder *gob.Encoder, value any, logger *log.Logger) {
	if err := encoder.Encode(value); err != nil {
		logger.Fatalln(err)
	}
}

// - Serializing
func EncodeData(logger *log.Logger, file_name string) {
	file, err := os.Create(file_name)
	if err != nil {
		logger.Fatalln(err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)

	EncodeWrapper(encoder, global_version, logger)
	EncodeWrapper(encoder, global_local_storage_key, logger)
	EncodeWrapper(encoder, global_people, logger)

	logger.Printf("saved %d people.\n", len(global_people))
}

func DecodeWrapper(decoder *gob.Decoder, value any, logger *log.Logger) {
	if err := decoder.Decode(value); err != nil {
		logger.Fatalln(err)
	}
}

func DecodeData(logger *log.Logger, file_name string) {
	file, err := os.Open(file_name)
	if errors.Is(err, os.ErrNotExist) {
  logger.Println("Datafile does not exist:", file_name)
	} else if err != nil {
		logger.Fatalln(err)
	} else {
		decoder := gob.NewDecoder(file)

		var version int
		DecodeWrapper(decoder, &version, logger)
		logger.Printf("datafile@%d program@%d\n", version, global_version)

		// NOTE(luca): this will automatically migrate v1 to v2
		if version == 2 {
			DecodeWrapper(decoder, &global_local_storage_key, logger)
			global_local_storage_key_initialized = true
		}

		DecodeWrapper(decoder, &global_people, logger)

		logger.Printf("Imported %d people.\n", len(global_people))

		global_people_initialized = true

		if err := file.Close(); err != nil {
			logger.Fatalln(err)
		}
	}
}

// - Person
func (person Person) String() string {
	var digits string
	if person.Token > 99999 {
		digits = fmt.Sprintf("%d", person.Token)[:6]
	} else {
		digits = fmt.Sprintf("%d", person.Token)
	}

	return fmt.Sprintf("%s_%s(%t)", person.Name, digits, person.HasPicked)
}

func TemplateToString(page_html string, people []Person, seed int64, internal bool) string {
	var buf strings.Builder
	template_response, err := template.New("tirage").Parse(page_html)
	if err != nil {
		fmt.Println(err)
	}

	type PageData struct {
		People   []Person
		Key      int64
		Internal bool
	}
	err = template_response.ExecuteTemplate(&buf, "tirage", PageData{People: people, Key: global_local_storage_key, Internal: internal})
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

func HttpError(logger *log.Logger, message string, person *Person, writer http.ResponseWriter, request *http.Request) {
	logger.Printf("Error for %s: %s | %s %s %s %s\n",
		person.Name, message,
		request.RemoteAddr, request.Method, request.URL, request.Form)
	http.Error(writer, message, http.StatusNotFound)
}

// - Main
func main() {
	var did_work bool
	var internal, slow bool
	var serve, shuffle, unpickall, show_people, reset_tokens bool
	var add_person, remove_person, unpick string
	var set_local_storage_key int64

	flag.BoolVar(&serve, "serve", false, "run http server")
	flag.BoolVar(&internal, "internal", false, "run commands in internal mode")
	flag.BoolVar(&slow, "slow", false, "run commands in slow mode")
	flag.BoolVar(&shuffle, "shuffle", false, "shuffle people again")
	flag.BoolVar(&reset_tokens, "reset_tokens", false, "reset tokens of people")
	flag.BoolVar(&unpickall, "unpickall", false, "unpick all people")
	flag.BoolVar(&show_people, "show_people", false, "show people")
	flag.StringVar(&add_person, "add_person", "", "add person by name")
	flag.StringVar(&remove_person, "remove_person", "", "remove person by name")
	flag.StringVar(&unpick, "unpick", "", "unpick person by name")
	flag.Int64Var(&set_local_storage_key, "set_local_storage_key", 0, "Set the local storage key")

	flag.Parse()

	logger := log.New(os.Stdout, "[noel] ", log.Ldate|log.Ltime)
	var seed int64
	if internal {
		seed = rand.Int63()
	} else {
		seed = 7967946373046491984
	}
	logger.Println("seed:", seed)
	source := rand.NewSource(seed)
	seeded_rand := rand.New(source)
	rand.Seed(seed)

	// Init people
	{
		DecodeData(logger, global_data_file_name)
		if !global_people_initialized {
			logger.Println("Initialize people.")
			ShufflePeople(seeded_rand, global_people, logger)

			for index := range global_people {
				// NOTE(luca): since javascript cannot handle big numbers we crop them
				global_people[index].Token = seeded_rand.Int63() / 10000
			}

			global_people_initialized = true
		}

		if !global_local_storage_key_initialized {
   logger.Println("Initialize local storage key.")
			global_local_storage_key = rand.Int63()
			global_local_storage_key_initialized = true
		}
	}

	if len(add_person) > 0 {
  global_people = append(global_people, Person{Name:add_person})
  fmt.Println(global_people)
		did_work = true
	}

	if len(remove_person) > 0 {
  for index, person := range global_people {
   if person.Name == remove_person {
    global_people = append(global_people[:index], global_people[index+1:]...)
    break
   }
  }
		logger.Println("remove:", remove_person)
		did_work = true
	}

	if reset_tokens {
		for index := range global_people {
			// NOTE(luca): since javascript cannot handle big numbers we crop them
			global_people[index].Token = seeded_rand.Int63() / 10000
		}

		logger.Println("reset tokens.")
		did_work = true
	}

	if unpickall {
		for index := range global_people {
			global_people[index].HasPicked = false
		}
		logger.Println("Unpicked all.")
		did_work = true
	}

	if shuffle {
		ShufflePeople(seeded_rand, global_people, logger)
		logger.Println("Shuffled people.")
		did_work = true
	}

	if show_people {
		for _, person := range global_people {
			fmt.Printf("%12s [%d] %t\n", person.Name, person.Token, person.HasPicked)
		}
	}

	if len(unpick) > 0 {
		logger.Println("unpick:", unpick)
		found, person := FindPersonByName(global_people, unpick)
		if found {
			person.HasPicked = false
			logger.Println(person)
		} else {
			logger.Fatalln("No such person")
		}
		did_work = true
	}

	if set_local_storage_key != 0 {
		global_local_storage_key = set_local_storage_key
		global_local_storage_key_initialized = true
		did_work = true
	}

	if serve {
		logger.Println("local storage key:", global_local_storage_key)
		logger.Println(global_people)

		go func() {
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)
			<-c

			EncodeData(logger, global_data_file_name)

			logger.Println("data saved.")
			os.Exit(0)
		}()

		http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./assets"))))

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

						fmt.Fprintln(writer, "ok")
					} else {
						HttpError(logger, "invalid token", person, writer, request)
					}
				} else {
					HttpError(logger, "no such person", &nil_person, writer, request)
				}
			} else if request.Method == http.MethodGet {
				params := request.URL.Query()
				name := params.Get("name")
				token := params.Get("token")

				found, person := FindPersonByName(global_people, name)

				if found {
					tokenString := strconv.FormatInt(person.Token, 10)

					if token == tokenString {
						fmt.Fprint(writer, global_people[person.Other].Wishlist)
					} else {
						HttpError(logger, "invalid token", person, writer, request)
					}
				} else {
					HttpError(logger, "no such person", &nil_person, writer, request)
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
						Token         int64
						ThisWishlist  string
						OtherName     string
						OtherWishlist string
					}
					other_person := global_people[person.Other]

					response := Response{person.Token, person.Wishlist, other_person.Name, other_person.Wishlist}

					json.NewEncoder(writer).Encode(response)
				} else {
					HttpError(logger, "person already picked", person, writer, request)
				}
			} else {
				HttpError(logger, "no such person", &nil_person, writer, request)
			}
		})

		// Execute the template before-hand since the contents won't change.
		response := TemplateToString(page_html, global_people, seed, internal)
		http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			if slow {
				buffer, err := os.ReadFile("index.tmpl.html")
				if err == nil {
					file_contents := string(buffer)
					response = TemplateToString(file_contents, global_people, seed, internal)
				} else {
					logger.Println(err)
				}
			}

			fmt.Fprint(writer, response)
		})

		var address string
		if internal {
			address = "0.0.0.0:15118"
		} else {
			address = "localhost:15118"
		}
		logger.Printf("Listening on http://%s\n", address)
		if err := http.ListenAndServe(address, nil); err != nil {
			panic(err)
		}

		did_work = true
	}

	if did_work {
		EncodeData(logger, global_data_file_name)
	}

	return
}
