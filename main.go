package main

//- Libraries
import "fmt"
import "net/http"
import "math/rand"
import "html/template"
import "strings"
import _ "embed"

//- Types
type Person struct {
	Name   string
	Other  int
}

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

//- Main
func main() {
 var seed int64
 seed = rand.Int63()
 seed = 5642410750512497522
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
   if(v == i) {
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
 var buf strings.Builder
 template_response, err := template.New("roulette").Parse(page_html)
 if err != nil {
  fmt.Println(err)
 }
 template_response.ExecuteTemplate(&buf, "roulette", people)
 response := buf.String()

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
  fmt.Fprint(writer, response)
	})

	address := "localhost:15118"
	fmt.Printf("Listening on http://%s\n", address)
	err = http.ListenAndServe(address, nil)
	if err != nil {
		panic(err)
	}

	return
}
