// Documentation
//
// Secret santa app that's random so people don't have to worry about the picking process.
// No emails or login, just use the domain.
//
// Run it with `go run .`
//
//- TODOs
// TODO(luca): De-Duplicate backups
// - Scan backups folder, find duplicates and deduplicate them?
// - non-issue if compressed?
//
// TODO(luca): Different devices support:
// - People want to access the app from multiple devices
// - There must be an easier way to do this than copy a 16digit long token (which they need)
//
// - One-Time-Code, generated from logged in device
//   - To prevent people from hacking it: -> if one wrong code is sent a new one must be requested
//   - Has the same effect as `/choose` on another device
//
// TODO(luca): If someone's want to access from another device his wishlist won't be synced,
// so we must provide a way to get your own wishlist.
//
// TODO(luca): Reimplement missing features.
// Since we only have one page we lose following functionality of the browser.
// - native navigation through history of urls
// - control+click to open in a new page

package noelan

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
	"io"
	"log"
	"os"
	"time"
)

// - Types
type Person struct {
	Name      string
	Other     int
	Wishlist  string
	HasPicked bool
	Token     int64
}

// - Globals
//
//go:embed index.tmpl.html
var GlobalPageHTML string

const GlobalDataFileName = "people.gob"
const GlobalDataDirectoryName = "gobs"
const GlobalPerson int = 2

var GlobalNilPerson = Person{}

// - Serializing
func EncodeWrapper(encoder *gob.Encoder, value any, logger *log.Logger) {
	if err := encoder.Encode(value); err != nil {
		logger.Fatalln(err)
	}
}

func EncodeData(logger *log.Logger, file_name string, version int, local_storage_key int64, people []Person) {

	file, err := os.Create(file_name)
	if err != nil {
		logger.Fatalln(err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)

	EncodeWrapper(encoder, version, logger)
	EncodeWrapper(encoder, local_storage_key, logger)
	EncodeWrapper(encoder, people, logger)

	logger.Printf("saved %d people.\n", len(people))
}

func DecodeWrapper(decoder *gob.Decoder, value any, logger *log.Logger) {
	if err := decoder.Decode(value); err != nil {
		logger.Fatalln(err)
	}
}

func DecodeData(logger *log.Logger, file_name string, version int, people *[]Person, local_storage_key *int64) (bool, bool) {
	local_storage_key_decoded := false
	people_decoded := false

	file, err := os.Open(file_name)
	if errors.Is(err, os.ErrNotExist) {
		logger.Println("No data imported.")
	} else if err != nil {
		logger.Fatalln(err)
	} else {
		decoder := gob.NewDecoder(file)

		var file_version int
		DecodeWrapper(decoder, &file_version, logger)
		logger.Printf("datafile@%d program@%d\n", file_version, version)

		// NOTE(luca): this will automatically migrate v1 to v2
		if file_version == 2 {
			DecodeWrapper(decoder, local_storage_key, logger)
			local_storage_key_decoded = true
		}

		DecodeWrapper(decoder, people, logger)

		logger.Printf("Imported %d people.\n", len(*people))
		people_decoded = true

		if err := file.Close(); err != nil {
			logger.Fatalln("Error closing file", err)
		}
	}

	return people_decoded, local_storage_key_decoded
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

// - Helpers
func TemplateToString(template_contents string, people []Person, local_storage_key int64, internal bool) string {
	var buf strings.Builder
	template_response, err := template.New("tirage").Parse(template_contents)
	if err != nil {
		fmt.Println(err)
	}

	type PageData struct {
		People   []Person
		Key      int64
		Internal bool
	}
	err = template_response.ExecuteTemplate(&buf, "tirage", PageData{People: people, Key: local_storage_key, Internal: internal})
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
func Run() {
	var people = []Person{
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
	var local_storage_key int64
	var people_initialized = false
	var local_storage_key_initialized = false

	// Flags
	var did_work bool
	var internal, slow, save bool
	var serve, shuffle, unpickall, show_people, reset_tokens bool
	var add_person, remove_person, unpick string
	var set_local_storage_key int64

	flag.BoolVar(&internal, "internal", false, "run commands in internal mode")
	flag.BoolVar(&slow, "slow", false, "run commands in slow mode")
	flag.BoolVar(&save, "save", false, "force saving to data file")
	flag.BoolVar(&shuffle, "shuffle", false, "shuffle people again")
	flag.BoolVar(&reset_tokens, "reset_tokens", false, "reset tokens of people")
	flag.BoolVar(&unpickall, "unpickall", false, "unpick all people")
	flag.BoolVar(&show_people, "show_people", false, "show people")
	flag.StringVar(&add_person, "add_person", "", "add person by name")
	flag.StringVar(&remove_person, "remove_person", "", "remove person by name")
	flag.StringVar(&unpick, "unpick", "", "unpick person by name")
	flag.Int64Var(&set_local_storage_key, "set_local_storage_key", 0, "Set the local storage key")
	flag.BoolVar(&serve, "serve", false, "run http server")

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

	// backup
	{
		file, err := os.Open(GlobalDataFileName)
		defer file.Close()
		if errors.Is(err, os.ErrNotExist) {
			logger.Printf("Cannot create backup: file '%s' not found.\n", GlobalDataFileName)
		} else if err != nil {
			logger.Fatalln(err)
		} else {
			_, err := os.Stat(GlobalDataDirectoryName)
			if os.IsNotExist(err) {
				err = os.MkdirAll(GlobalDataDirectoryName, 0755)
				if err != nil {
					logger.Printf("Error creating directory: %v\n", err)
				}
				logger.Println("Directory created successfully")
			} else if err != nil {
				logger.Printf("Error checking directory: %v\n", err)
			} else {
				// Directory already exists
			}

			now := time.Now()
			formatted_now := now.Format("060102_15_04_05")

			destination_file_name := fmt.Sprintf("./%s/%s__%s", GlobalDataDirectoryName, formatted_now, GlobalDataFileName)
			destination, err := os.Create(destination_file_name)
			if err != nil {
				logger.Fatalln("Error creating copy:", err)
			}
			defer destination.Close()

			_, err = io.Copy(destination, file)
			if err == nil {
				logger.Println("Created backup.")
			} else {
				logger.Fatalln("Error creating backup:", err)
			}
		}
	}

	// Init people
	{
		people_initialized, local_storage_key_initialized =
			DecodeData(logger, GlobalDataFileName, GlobalPerson, &people, &local_storage_key)
		if !people_initialized {
			logger.Println("Initialize people.")
			ShufflePeople(seeded_rand, people, logger)

			for index := range people {
				// NOTE(luca): since javascript cannot handle big numbers we crop them
				people[index].Token = seeded_rand.Int63() / 10000
			}

			people_initialized = true
		}

	}

	if save {
		did_work = true
	}

	if len(add_person) > 0 {
		people = append(people, Person{Name: add_person})
		fmt.Println(people)
		did_work = true
	}

	if len(remove_person) > 0 {
		for index, person := range people {
			if person.Name == remove_person {
				people = append(people[:index], people[index+1:]...)
				break
			}
		}
		logger.Println("remove:", remove_person)
		did_work = true
	}

	if reset_tokens {
		did_work = true
		// NOTE(luca): Users will need to pick again
		local_storage_key_initialized = false

		for index := range people {
			// NOTE(luca): since javascript cannot handle big numbers we crop them
			people[index].Token = seeded_rand.Int63() / 10000
		}

		logger.Println("reset tokens.")
	}

	if unpickall {
		did_work = true
		for index := range people {
			people[index].HasPicked = false
		}
		logger.Println("Unpicked all.")
	}

	if shuffle {
		did_work = true
		ShufflePeople(seeded_rand, people, logger)
		logger.Println("Shuffled people.")
	}

	if show_people {
		for _, person := range people {
			fmt.Printf("%12s [%15d] %t\n", person.Name, person.Token, person.HasPicked)
		}
	}

	if len(unpick) > 0 {
		did_work = true
		logger.Println("unpick:", unpick)
		found, person := FindPersonByName(people, unpick)
		if found {
			person.HasPicked = false
			logger.Println(person)
		} else {
			logger.Fatalln("No such person")
		}
	}

	if set_local_storage_key != 0 {
		did_work = true
		local_storage_key = set_local_storage_key
		local_storage_key_initialized = true
	}

	if !local_storage_key_initialized {
		logger.Println("Initialize local storage key.")
		local_storage_key = rand.Int63()
		local_storage_key_initialized = true
	}

	if serve {
		did_work = true
		logger.Println("local storage key:", local_storage_key)
		logger.Println(people)

		go func() {
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt)
			<-c

			if did_work {
				EncodeData(logger, GlobalDataFileName, GlobalPerson, local_storage_key, people)
			}

			logger.Println("data saved.")
			os.Exit(0)
		}()

		http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./assets"))))

		http.HandleFunc("/api/list/", func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodPost {
				name := request.FormValue("name")
				text := request.FormValue("text")
				token := request.FormValue("token")

				logger.Println("Edit wishlist of", name)
				found, person := FindPersonByName(people, name)

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
					HttpError(logger, "no such person", &GlobalNilPerson, writer, request)
				}
			} else if request.Method == http.MethodGet {
				params := request.URL.Query()
				name := params.Get("name")
				token := params.Get("token")

				found, person := FindPersonByName(people, name)

				if found {
					tokenString := strconv.FormatInt(person.Token, 10)

					if token == tokenString {
						fmt.Fprint(writer, people[person.Other].Wishlist)
					} else {
						HttpError(logger, "invalid token", person, writer, request)
					}
				} else {
					HttpError(logger, "no such person", &GlobalNilPerson, writer, request)
				}
			}
		})

		http.HandleFunc("/api/choose/", func(writer http.ResponseWriter, request *http.Request) {
			params := request.URL.Query()
			name := params.Get("name")
			found, person := FindPersonByName(people, name)
			if found {
				if !person.HasPicked {
					person.HasPicked = true

					type Response struct {
						Token         int64
						ThisWishlist  string
						OtherName     string
						OtherWishlist string
					}
					other_person := people[person.Other]

					response := Response{person.Token, person.Wishlist, other_person.Name, other_person.Wishlist}

					json.NewEncoder(writer).Encode(response)
				} else {
					HttpError(logger, "person already picked", person, writer, request)
				}
			} else {
				HttpError(logger, "no such person", &GlobalNilPerson, writer, request)
			}
		})

		// Execute the template before-hand since the contents won't change.
		response := TemplateToString(GlobalPageHTML, people, local_storage_key, internal)
		http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			if slow {
				buffer, err := os.ReadFile("./code/index.tmpl.html")
				if err == nil {
					file_contents := string(buffer)
					response = TemplateToString(file_contents, people, local_storage_key, internal)
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

	}

	if did_work {
		EncodeData(logger, GlobalDataFileName, GlobalPerson, local_storage_key, people)
	}

	return
}
