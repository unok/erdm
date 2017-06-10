package main

import (
	"fmt"
	"os"
	"log"
	"io/ioutil"
	"reflect"
	"text/template"
	"os/exec"
	"strings"
	"flag"
	"path/filepath"
	"path"
)

type TableRelation struct {
	TableNameReal          string
	CardinalitySource      string
	CardinalityDestination string
}

type Index struct {
	Title    string
	Columns  []string
	IsUnique bool
}

type Column struct {
	TitleReal    string
	Title        string
	Type         string
	AllowNull    bool
	IsUnique     bool
	IsPrimaryKey bool
	IsForeignKey bool
	Default      string
	Relation     TableRelation
	Comments     []string
	IndexIndexes []int
	WithoutErd   bool
}

type Table struct {
	TitleReal       string
	Title           string
	Columns         []Column
	CurrentColumnId int
	PrimaryKeys     []int
	Indexes         []Index
	CurrentIndexId  int
}

type ErdM struct {
	Title          string
	Tables         []Table
	CurrentTableId int
	ImageFilename  string
	IsError        bool
}

func openFile(filename string) *os.File {
	fp, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	return fp
}

func readAll(fp *os.File) []byte {
	bs, err := ioutil.ReadAll(fp)
	if err != nil {
		log.Fatal(err)
	}
	return bs
}

func (e *ErdM) setTitle(t string) {
	e.Title = t
}

func (e *ErdM) addTableTitleReal(t string) {
	e.Tables = append(e.Tables, Table{TitleReal: t})
	e.CurrentTableId = len(e.Tables) - 1
}

func (e *ErdM) addTableTitle(t string) {
	t = strings.Trim(t, "\"")
	e.Tables[e.CurrentTableId].Title = t
}

func (e *ErdM) addPrimaryKey(text string) {
	if (len(text) > 0) {
		if (in_array(0, e.Tables[e.CurrentTableId].PrimaryKeys)) {
			e.Tables[e.CurrentTableId].PrimaryKeys = append(e.Tables[e.CurrentTableId].PrimaryKeys, 1)
		} else {
			e.Tables[e.CurrentTableId].PrimaryKeys = append(e.Tables[e.CurrentTableId].PrimaryKeys, 0)
		}
	} else {
		e.Tables[e.CurrentTableId].PrimaryKeys = append(e.Tables[e.CurrentTableId].PrimaryKeys, e.Tables[e.CurrentTableId].CurrentColumnId + 1)
	}
}

func (e *ErdM) setColumnNameReal(t string) {
	e.Tables[e.CurrentTableId].Columns = append(e.Tables[e.CurrentTableId].Columns, Column{TitleReal: t, AllowNull: true, IsUnique: false, IsForeignKey: false, WithoutErd: false})
	e.Tables[e.CurrentTableId].CurrentColumnId = len(e.Tables[e.CurrentTableId].Columns) - 1
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].IsPrimaryKey = e.Tables[e.CurrentTableId].isPrimaryKey(e.Tables[e.CurrentTableId].CurrentColumnId)
}

func (e *ErdM) setColumnName(t string) {
	t = strings.Trim(t, "\"")
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].Title = t
}

func (e *ErdM) addColumnType(t string) {
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].Type = t
}

func (e *ErdM) setNotNull() {
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].AllowNull = false
}
func (e *ErdM) setUnique() {
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].IsUnique = true
}

func (e *ErdM) setColumnDefault(t string) {
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].Default = t
}
func (e *ErdM) setWithoutErd() {
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].WithoutErd = true
}
func (e *ErdM) setRelationSource(t string) {
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].Relation.CardinalitySource = t
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].IsForeignKey = true
}

func (e *ErdM) setRelationDestination(t string) {
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].Relation.CardinalityDestination = t
}

func (e *ErdM) setRelationTableNameReal(t string) {
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].Relation.TableNameReal = t
}

func (e *ErdM) addComment(t string) {
	e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].Comments = append(e.Tables[e.CurrentTableId].Columns[e.Tables[e.CurrentTableId].CurrentColumnId].Comments, t)
}

func (e *ErdM) setIndexName(t string) {
	e.Tables[e.CurrentTableId].Indexes = append(e.Tables[e.CurrentTableId].Indexes, Index{Title: t, IsUnique: false})
	e.Tables[e.CurrentTableId].CurrentIndexId = len(e.Tables[e.CurrentTableId].Indexes) - 1
}

func (e *ErdM) setUniqueIndex() {
	e.Tables[e.CurrentTableId].Indexes[e.Tables[e.CurrentTableId].CurrentIndexId].IsUnique = true
}

func (e *ErdM) setIndexColumn(t string) {
	e.Tables[e.CurrentTableId].Indexes[e.Tables[e.CurrentTableId].CurrentIndexId].Columns = append(e.Tables[e.CurrentTableId].Indexes[e.Tables[e.CurrentTableId].CurrentIndexId].Columns, t)
	i, err := e.Tables[e.CurrentTableId].getColumnIndex(t)
	if err != nil {
		fmt.Println(err)
	}
	e.Tables[e.CurrentTableId].Columns[i].IndexIndexes = append(e.Tables[e.CurrentTableId].Columns[i].IndexIndexes, e.Tables[e.CurrentTableId].CurrentIndexId)
}

func (t *Table) getColumnIndex(s string) (int, error) {
	for i, v := range t.Columns {
		if v.TitleReal == s {
			return i, nil
		}
	}
	return -1, os.ErrInvalid
}

func in_array(val interface{}, array interface{}) (exists bool) {
	exists = false

	switch reflect.TypeOf(array).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(array)

		for i := 0; i < s.Len(); i++ {
			if reflect.DeepEqual(val, s.Index(i).Interface()) == true {
				exists = true
				return
			}
		}
	}

	return
}

func (t *Table) isPrimaryKey(index int) bool {
	return in_array(index, t.PrimaryKeys)
}

func (c *Column) HasDefaultSetting() bool {
	return len(c.Default) > 0
}

func (c *Column) HasRelation() bool {
	return len(c.Relation.TableNameReal) > 0
}

func (c *Column) HasComment() bool {
	return len(c.Comments) > 0
}

func (t *Table) GetPrimaryKeyColumns() string {
	ps := []string{}
	for _, pk := range t.PrimaryKeys {
		ps = append(ps, t.Columns[pk].TitleReal)
	}
	return strings.Join(ps, ", ");
}

func (i *Index) GetIndexColumns() string {
	return strings.Join(i.Columns, ", ");
}

func (c *ErdM) Err(pos int, buffer string) {
	fmt.Println("")
	a := strings.Split(buffer[:pos], "\n")
	row := len(a) - 1
	column := len(a[row]) - 1

	lines := strings.Split(buffer, "\n")
	for i := row - 5; i <= row; i++ {
		if i < 0 {
			i = 0
		}

		fmt.Println(lines[i])
	}

	s := ""
	for i := 0; i <= column; i++ {
		s += " "
	}
	ln := len(strings.Trim(lines[row], " \r\n"))
	for i := column + 1; i < ln; i++ {
		s += "~"
	}
	fmt.Println(s)

	fmt.Println("error")
	c.IsError = true
}

func main() {
	// check dot command
	dot_err := exec.Command("dot", "-?").Run()
	if dot_err != nil {
		fmt.Println(dot_err)
		fmt.Println("Please check a graphviz(dot) setting.")
		return
	}

	usage := "Usage: erdm [-output_dir directory_name] erd.erdm"

	// check arguments
	wd, _ := os.Getwd()
	output_dir := flag.String("output_dir", wd, "output directory")
	flag.Parse()
	if len(flag.Args()) == 0 {
		fmt.Println(usage)
		return
	}
	input_file := flag.Args()[0]
	stat, err := os.Stat(input_file)
	if err != nil {
		fmt.Println(err)
		fmt.Println(usage)
		return
	}
	if stat.IsDir() == true {
		fmt.Println("Please set inputfile: " + input_file)
		fmt.Println(usage)
		return
	}

	stat, err = os.Stat(*output_dir)
	if err != nil {
		fmt.Println(err)
		fmt.Println(usage)
		return
	}
	if stat.IsDir() != true {
		fmt.Println("Please set output_dir: " + *output_dir)
		fmt.Println(usage)
		return
	}

	f := filepath.Base(input_file)
	basename := f[:len(f) - len(path.Ext(f))]

	fp := openFile(input_file)
	content := readAll(fp)
	parser := &Parser{Buffer: string(content)}
	parser.Init()
	err = parser.Parse()
	if err != nil {
		fmt.Println(err)
		return
	}
	parser.Execute()

	dot_string, err := Asset("templates/dot.tmpl")
	if err != nil {
		fmt.Println(err)
		return
	}
	dot_tables_string, err := Asset("templates/dot_tables.tmpl")
	if err != nil {
		fmt.Println(err)
		return
	}
	dot_relations_string, err := Asset("templates/dot_relations.tmpl")
	if err != nil {
		fmt.Println(err)
		return
	}
	html_string, err := Asset("templates/html.tmpl")
	if err != nil {
		fmt.Println(err)
		return
	}
	pg_ddl_string, err := Asset("templates/pg_ddl.tmpl")
	if err != nil {
		fmt.Println(err)
		return
	}
	sqlite3_ddl_string, err := Asset("templates/sqlite3_ddl.tmpl")
	if err != nil {
		fmt.Println(err)
		return
	}
	t, err := template.New("template").Parse(string(dot_string) + string(dot_tables_string) + string(dot_relations_string) + string(html_string) + string(pg_ddl_string) + string(sqlite3_ddl_string))
	// "templates/dot.tmpl", "templates/dot_tables.tmpl", "templates/dot_relations.tmpl", "templates/html.tmpl", "templates/pg_ddl.tmpl")
	if err != nil {
		fmt.Println(err)
		return
	}

	dot_filename := path.Join(*output_dir, basename + ".dot")
	_, err = os.Stat(dot_filename)
	if err == nil {
		if err = os.Remove(dot_filename); err != nil {
			fmt.Println(err)
			return
		}
	}
	fp, err = os.OpenFile(dot_filename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = t.ExecuteTemplate(fp, "dot", parser.ErdM)
	if err != nil {
		fmt.Println(err)
		return
	}
	png_filename := path.Join(*output_dir, basename + ".png")
	err = exec.Command("dot", "-T", "png", "-o", png_filename, dot_filename).Run()
	if err != nil {
		fmt.Println(err)
		return
	}
	parser.ErdM.ImageFilename = path.Base(png_filename)

	html_filename := path.Join(*output_dir, basename + ".html")
	_, err = os.Stat(html_filename)
	if err == nil {
		if err = os.Remove(html_filename); err != nil {
			fmt.Println(err)
			return
		}
	}
	fp, err = os.OpenFile(html_filename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = t.ExecuteTemplate(fp, "html", parser.ErdM)
	if err != nil {
		fmt.Println(err)
		return
	}

	pgsql_filename := path.Join(*output_dir, basename + ".pg.sql")
	_, err = os.Stat(pgsql_filename)
	if err == nil {
		if err = os.Remove(pgsql_filename); err != nil {
			fmt.Println(err)
			return
		}
	}
	fp, err = os.OpenFile(pgsql_filename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = t.ExecuteTemplate(fp, "pg_ddl", parser.ErdM)
	if err != nil {
		fmt.Println(err)
		return
	}

	sqlite3_filename := path.Join(*output_dir, basename + ".sqlite3.sql")
	_, err = os.Stat(sqlite3_filename)
	if err == nil {
		if err = os.Remove(sqlite3_filename); err != nil {
			fmt.Println(err)
			return
		}
	}
	fp, err = os.OpenFile(sqlite3_filename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = t.ExecuteTemplate(fp, "sqlite3_ddl", parser.ErdM)
	if err != nil {
		fmt.Println(err)
		return
	}
}
