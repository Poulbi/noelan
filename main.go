package main

//- Libraries
import "fmt"
import "net/http"
import "math/rand"
import "html/template"
import "strings"
import _ "embed"

// - Types
type Person struct {
	Name  string
	Other int
}

type PageData struct {
 People []Person
 Seed int64
}

var DEBUG = true

//- Globals

//go:embed index.tmpl.html
var page_html string

var people = []Person{
	{Name: "Nawel"},
	{Name: "Tobias"},
	{Name: "Luca"},
	{Name: "Aeris"},
	{Name: "Lionel"},
	{Name: "Aurélie"},
	{Name: "Sean"},
	{Name: "Émilie"},
	{Name: "Yves"},
	{Name: "Marthe"},
}
var people_count = len(people)

func TemplateToString(page_html string, people []Person, seed int64) string {
			var buf strings.Builder
			template_response, err := template.New("roulette").Parse(page_html)
			if err != nil {
				fmt.Println(err)
			}
   err = template_response.ExecuteTemplate(&buf, "roulette", PageData{people, seed})
   if err != nil {
    fmt.Println(err)
   }
			response := buf.String()

   return response
}

// - Main
func main() {
	var seed int64
	seed = rand.Int63()
	seed = 1924480304604450476
	fmt.Println("seed:", seed)

	src := rand.NewSource(seed)
	r := rand.New(src)
	rand.Seed(seed)

	var list []int
	correct := false
	for !correct {
		list = r.Perm(people_count)

		correct = true
		for i, v := range list {
			if v == i {
				fmt.Println("incorrect, need to reshuffle")
				correct = false
				break
			}
		}
	}

	for i, v := range list {
		people[i].Other = v
	}

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./assets"))))

	http.HandleFunc("/person/", func(writer http.ResponseWriter, request *http.Request) {
		name := request.FormValue("name")

		var found bool
		for _, value := range people {
			if name == value.Name {
				found = true
			}
		}

		if found {
			fmt.Fprintln(writer, "ok")
		} else {
			fmt.Fprintln(writer, "error")
		}
	})

	// Execute the template before-hand since the contents won't change.

	response := TemplateToString(page_html, people, seed);

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		if DEBUG {
   response = TemplateToString(page_html, people, seed);
		}
  fmt.Fprint(writer, response)
	})

	var address string
	if DEBUG {
		address = "0.0.0.0:15118"
	} else {
		address = "localhost:15118"
	}
	fmt.Printf("Listening on http://%s\n", address)
 if err := http.ListenAndServe(address, nil); err != nil {
		panic(err)
	}

	return
}
