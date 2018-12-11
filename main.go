package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rs/cors"

	"github.com/gorilla/mux"
)

var flagUser string
var flagPass string

type Document struct {
	ID   string
	Name string
	Size int
}

type DocumentDAO struct {
	ID   string
	Name string
	Size int
	Path string
}

type User struct {
	ID    string
	Name  string
	Email string
}

var docs map[string]DocumentDAO
var users map[string]User

func main() {
	router := mux.NewRouter()
	docs = make(map[string]DocumentDAO)
	users = make(map[string]User)

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},                            // All origins
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"}, // Allowing only get, just an example
	})

	flagUser = "user"
	flagPass = "pass"
	router.HandleFunc("/documents", use(getDocuments, basicAuth)).Methods("GET")
	router.HandleFunc("/uploadDocument/{id}", use(uploadDocumentWithUser, basicAuth)).Methods("POST")
	router.HandleFunc("/deleteDocuments/{id}/{userid}", use(deleteDocumentsWithUser, basicAuth)).Methods("DELETE")

	router.HandleFunc("/users", use(createUsers, basicAuth)).Methods("POST")

	log.Fatal(http.ListenAndServe(":9000", c.Handler(http.TimeoutHandler(router, time.Second*10, "Timeout!"))))
}

func uploadDocument(w http.ResponseWriter, r *http.Request) string {
	r.ParseMultipartForm(32 << 20)
	file, handler, err := r.FormFile("file")
	if err != nil {
		fmt.Println("no pudo cargar el file")
		fmt.Println(err)
		http.Error(w, "", http.StatusInternalServerError)
		return ""
	}
	defer file.Close()
	f, err := os.OpenFile("./storeFile/"+handler.Filename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("no pudo escribir el file")
		http.Error(w, "", http.StatusInternalServerError)
		return ""
	}
	defer f.Close()
	io.Copy(f, file)

	return handler.Filename

}

func uploadDocumentWithUser(w http.ResponseWriter, r *http.Request) {
	//descomentar esto para que suba el documento
	r.ParseMultipartForm(32 << 20)
	file, handler, err := r.FormFile("file")
	if err != nil {
		fmt.Println("no pudo cargar el file")
		fmt.Println(err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, file); err != nil {
		fmt.Println(err)
		return
	}
	//name := uploadDocument(w, r)
	name := SaveStorage(handler.Filename, buf.Bytes())
	fmt.Println("nombre del archivo: ", handler.Filename)
	//agregar codigo para notificar sobre que usuario subió
	vars := mux.Vars(r)
	_, user, users := getUserAndRestOfUsers(vars["id"])
	//enviar a la cola de correo

	if name != "ERROR" {
		SendMail(user, users, "Se ha subido el archivo "+name)
	}

}

func findUserByName(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	found, _, _ := getUserAndRestOfUsers(vars["id"])
	if !found {
		http.Error(w, "notFound", http.StatusNotFound)
	}
}

func deleteDocumentsWithUser(w http.ResponseWriter, r *http.Request) {
	//name := deleteDocuments(w, r)

	vars := mux.Vars(r)
	name := deleteStorage(vars["id"])
	fmt.Println("el deleteStorage devolvio :", name)
	_, user, users := getUserAndRestOfUsers(vars["userid"])
	if name != "ERROR" {
		SendMail(user, users, "Se ha eliminado el archivo "+name)
	}
}

func createUsers(w http.ResponseWriter, r *http.Request) {

	decoder := json.NewDecoder(r.Body)
	var t User
	err := decoder.Decode(&t)
	name := t.Name
	email := t.Email
	fmt.Print(name + " + " + email)

	//--------------
	root := "./users/users.txt"
	var buffer bytes.Buffer
	found := false
	lastId := 1
	file, err := ioutil.ReadFile(root)
	if err != nil {
		fmt.Print(err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	registers := strings.Split(string(file), ";")
	for _, reg := range registers[:] {
		aux := strings.Split(reg, ",")
		if len(aux) > 1 {
			buffer.WriteString(aux[0])
			buffer.WriteString(",")
			buffer.WriteString(aux[1])
			buffer.WriteString(",")
			buffer.WriteString(aux[2])
			buffer.WriteString(";")
			lastId, _ = strconv.Atoi(aux[0])
			lastId++
			if aux[1] == name && aux[2] == email {
				found = true
			}
		}
	}
	if !found {
		buffer.WriteString(strconv.Itoa(lastId))
		buffer.WriteString(",")
		buffer.WriteString(name)
		buffer.WriteString(",")
		buffer.WriteString(email)
		buffer.WriteString(";")
	}
	err = ioutil.WriteFile(root, []byte(buffer.String()), 0644)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
}

func getDocumentById(w http.ResponseWriter, r *http.Request) {
	//var docs []Document
	docs = loadDocuments(docs)
	vars := mux.Vars(r)

	w.Header().Set("Content-Type", "application/json")
	var doc DocumentDAO
	for _, v := range docs {
		if v.ID == vars["id"] {
			doc = v

		}
	}

	if documentInArray(vars["id"], docs) != "" {
		json.NewEncoder(w).Encode(parseDocument(doc))
	} else {
		http.Error(w, "", http.StatusNotFound)

	}

}

func documentInArray(a string, list map[string]DocumentDAO) string {
	for _, b := range list {
		if b.ID == a {
			return b.Name
		}
	}
	return ""
}

func getDocuments(w http.ResponseWriter, r *http.Request) {
	var docs map[string]DocumentDAO
	docs = make(map[string]DocumentDAO)
	docs = loadDocuments(docs)

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(parseDocuments(docs))
	//json.NewEncoder(w).Encode(docs)

}

func getUsers(w http.ResponseWriter, r *http.Request) {
	var users []User
	users = make([]User, 0)
	users = loadUsers(users)

	/*var users map[string]User
	users = make(map[string]User, 0)
	users = loadUsers(users)
	*/

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(users)
	//json.NewEncoder(w).Encode(docs)

}

func loadDocuments(docs map[string]DocumentDAO) map[string]DocumentDAO {
	/*root := "./temp/"
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.Name() != "temp" {
			id, error := hash_file_md5(path)
			if error == nil {
				//*docs = append(*docs,
				//	Document{ID: id, Name: info.Name(), Size: int(info.Size())})
				docs[id] = DocumentDAO{ID: id, Name: info.Name(), Size: int(info.Size()), Path: path}
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}*/
	data := make(map[string]DocumentDAO, 0)
	//data["test"] = DocumentDAO{ID: "1", Name: "name", Size: 1, Path: "path"}
	requestDoc := RequestGetDocuments{List: data}
	responseDoc := SendStorage(requestDoc)
	return responseDoc.List
}

/*func loadUsers(users map[string]User) map[string]User {
	root := "./users/users.txt"
	file, err := ioutil.ReadFile(root)
	if err != nil {
		fmt.Print(err)
	}
	registers := strings.Split(string(file), ";")
	for _, reg := range registers[:] {
		aux := strings.Split(reg, ",")
		if len(aux) > 1 {
			users[aux[0]] = User{ID: aux[0], Name: aux[1], Email: aux[2]}
		}
	}
	return users
}*/

func loadUsers(users []User) []User {
	root := "./users/users.txt"
	file, err := ioutil.ReadFile(root)
	if err != nil {
		fmt.Print(err)
	}
	registers := strings.Split(string(file), ";")
	for _, reg := range registers[:] {
		aux := strings.Split(reg, ",")
		if len(aux) > 1 {
			users = append(users, User{ID: aux[0], Name: aux[1], Email: aux[2]})
		}
	}
	return users
}

func deleteDocuments(w http.ResponseWriter, r *http.Request) string {
	//var docs []Document

	docs = loadDocuments(docs)
	vars := mux.Vars(r)

	w.Header().Set("Content-Type", "application/json")
	result := documentInArray(vars["id"], docs)
	if result != "" {
		deleteDocument(vars["id"])

	} else {
		http.Error(w, "", http.StatusNotFound)

	}
	return result

}

func deleteUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)

	if !deleteUser(vars["id"]) {
		http.Error(w, "", http.StatusNotFound)
	}
}

func deleteUser(userId string) bool {
	root := "./users/users.txt"
	var buffer bytes.Buffer
	resp := false
	file, err := ioutil.ReadFile(root)
	if err != nil {
		fmt.Print(err)
	}

	registers := strings.Split(string(file), ";")
	for _, reg := range registers[:] {
		aux := strings.Split(reg, ",")
		if len(aux) > 1 {
			if aux[0] != userId {
				buffer.WriteString(aux[0])
				buffer.WriteString(",")
				buffer.WriteString(aux[1])
				buffer.WriteString(",")
				buffer.WriteString(aux[2])
				buffer.WriteString(";")
			} else {
				resp = true
			}
		}
	}
	err = ioutil.WriteFile(root, []byte(buffer.String()), 0644)
	if err != nil {
		log.Fatalln(err)
	}
	return resp
}

func getUserAndRestOfUsers(name string) (bool, User, []User) {
	root := "./users/users.txt"
	found := false
	var users []User
	users = make([]User, 0)
	var user User
	file, err := ioutil.ReadFile(root)
	if err != nil {
		fmt.Print(err)
	}
	registers := strings.Split(string(file), ";")
	for _, reg := range registers[:] {
		aux := strings.Split(reg, ",")
		if len(aux) > 1 {
			if aux[1] != name {
				users = append(users, User{ID: aux[0], Name: aux[1], Email: aux[2]})
			} else {
				user = User{ID: aux[0], Name: aux[1], Email: aux[2]}
				found = true
			}

		}
	}
	return found, user, users
}

func deleteDocument(docId string) bool {
	root := "./temp/"
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.Name() != "temp" {
			id, error := hash_file_md5(path)
			if error == nil {
				if id == docId {
					os.Remove(path)
				}
			}
		}
		return nil
	})
	if err != nil {
		return true
	} else {
		return false
	}
}

func hash_file_md5(filePath string) (string, error) {
	var returnMD5String string
	file, err := os.Open(filePath)
	if err != nil {
		return returnMD5String, err
	}
	defer file.Close()
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return returnMD5String, err
	}
	hashInBytes := hash.Sum(nil)[:16]
	returnMD5String = hex.EncodeToString(hashInBytes)
	return returnMD5String, nil
}

func basicAuth(h http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()

		if flagUser != user || flagPass != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}

		h.ServeHTTP(w, r)
	})
}

func use(h http.HandlerFunc, middleware ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	/*for _, m := range middleware {
		h = m(h)
	}*/ //esto da cabeceras de seguridad
	return h
}

func serveDocuments(w http.ResponseWriter, r *http.Request) {
	//var docs []Document
	docs = loadDocuments(docs)
	vars := mux.Vars(r)
	var docPath string
	//w.Header().Set("Content-Type", "application/octet-stream")
	//w.Header().Set("Content-Disposition", "attachment")

	if documentInArray(vars["id"], docs) != "" {
		docPath = serveDocument(vars["id"])
		if docPath != "" {

			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", "attachment; filename="+docs[vars["id"]].Name)
			http.ServeFile(w, r, docPath)
		} else {
			http.Error(w, "", http.StatusInternalServerError)
		}
	} else {
		http.Error(w, "", http.StatusNotFound)

	}

}

func serveDocument(docId string) string {
	root := "./temp/"
	var docPath string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.Name() != "temp" {
			id, error := hash_file_md5(path)
			if error == nil {
				if id == docId {
					docPath = path
				}
			}
		}
		return nil
	})

	return docPath

}

func parseDocuments(dao map[string]DocumentDAO) []Document {
	var d []Document
	d = make([]Document, 0)
	for _, data := range dao {
		d = append(d, Document{ID: data.ID, Name: data.Name, Size: data.Size})
	}
	return d
}

func parseDocument(data DocumentDAO) Document {
	return Document{ID: data.ID, Name: data.Name, Size: data.Size}

}
