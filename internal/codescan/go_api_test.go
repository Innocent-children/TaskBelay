package codescan

import (
	"slices"
	"strings"
	"testing"
)

func entryNames(file *File) []string {
	var names []string
	for _, entry := range file.Entries {
		names = append(names, entry.Location())
	}
	slices.Sort(names)
	return names
}

func TestGoDocumentedEntryCallsUseImportsReceiversAndConstants(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"names.go": "package app\nconst FlagName = \"for\" + \"mat\"\nconst Route = \"/export\"\n",
		"main.go": `package app
import ( f "flag"; "os"; h "net/http"; "github.com/spf13/cobra"; c "github.com/urfave/cli/v2"; "github.com/go-chi/chi/v5"; "github.com/gin-gonic/gin"; "github.com/labstack/echo/v4" )
type Opt struct { Format string ` + "`yaml:\"format\" toml:\"format\"`" + `; Debug bool ` + "`mapstructure:\"debug\"`" + `; Secret string ` + "`yaml:\"-\"`" + ` }
func install() {
 f.String(FlagName,"csv","")
 fs:=f.NewFlagSet("app",f.ContinueOnError); var debug bool; fs.BoolVar(&debug,"debug",false,"")
 cmd:=&cobra.Command{Use:"export [path]"}; cmd.Flags().StringP("output","o","",""); cmd.PersistentFlags().Int("limit",0,"")
 _ = &c.StringFlag{Name:"format",Aliases:[]string{"f"}}; _ = &c.Command{Name:"serve"}
 _ = os.Getenv("HOME"); _,_ = os.LookupEnv("TMPDIR")
 h.HandleFunc(Route,nil); mux:=h.NewServeMux(); mux.Handle("/local",nil)
 r:=chi.NewRouter(); r.Get("/chi",nil); r.Method("POST","/post",nil)
 r.Route("/api",func(child chi.Router){child.Get("/received-chi",nil)})
 gin.Default().GET("/gin",nil); e:=echo.New(); e.GET("/echo",nil)
 group:=gin.New().Group("/v1"); group.GET("/csv",nil)
}
func withReceivers(f *f.FlagSet, mux *h.ServeMux){f.Int("size",0,"");mux.HandleFunc("/received",nil)}
`})
	file := snapshot.Files["main.go"]
	if len(snapshot.Missing) > 0 {
		t.Fatal(snapshot.Missing)
	}
	want := []string{"flag:--format", "flag:--debug", "flag:--output", "flag:-o", "flag:--limit", "flag:--format", "flag:-f", "flag:--size", "cmd:export", "cmd:serve", "env:HOME", "env:TMPDIR", "route:/export", "route:/local", "route:GET /chi", "route:POST /post", "route:GET /gin", "route:GET /echo", "route:GET /v1/csv", "route:/received", "route:/api", "route:GET /api/received-chi", "config:format", "config:debug"}
	slices.Sort(want)
	if got := entryNames(file); !slices.Equal(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	if !file.Operations["entries"].Complete || !file.CodeComplete {
		t.Fatal(file.Operations)
	}
	for _, entry := range file.Entries {
		if entry.Declaration == "" || entry.Span.StartLine == 0 || entry.Evidence == "" {
			t.Fatal(entry)
		}
	}
}

func TestGoDynamicEntryAndShadowingAreNotCertain(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.go": `package app
import("os";"flag";"github.com/gin-gonic/gin")
type Fake struct{}
func(Fake) Getenv(string){}
func(Fake) String(string,string,string){}
func f(dynamic string, unknown interface{}, flag Fake) {
 os.Getenv(dynamic); flag.String("not-a-flag","","")
 { os:=Fake{}; os.Getenv("not-an-env") }
 os.Getenv("VALID")
 unknown.Get("/unknown",nil)
 gin.New().Group(dynamic).GET("/unknown-prefix",nil)
}
`})
	file := snapshot.Files["a.go"]
	if got := entryNames(file); !slices.Equal(got, []string{"env:VALID"}) {
		t.Fatal(got)
	}
	if file.Operations["entries"].Complete || !file.CodeComplete {
		t.Fatal(file.Operations, file.Missing)
	}
	for _, reason := range []string{"字符串常量", "接收者类型", "前缀"} {
		found := false
		for _, missing := range file.Missing {
			found = found || strings.Contains(missing.Reason, reason)
		}
		if !found {
			t.Fatalf("missing %s: %+v", reason, file.Missing)
		}
	}
}

func TestGoDocumentedRisksAndUnrelatedNames(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.go": `package app
import("os";"database/sql";"encoding/json";"encoding/xml";"strconv";"fmt";"time";"net/url";"os/exec";"sync";"sync/atomic";"gopkg.in/yaml.v3";"github.com/pelletier/go-toml/v2")
type Fake struct{}
func(Fake) Write([]byte){}
func work(file *os.File, db *sql.DB, dec *json.Decoder, xdec *xml.Decoder, ch chan int){
 go func(){}(); ch<-1; <-ch; select{}; var mu sync.Mutex; mu.Lock(); atomic.AddInt32(nil,1)
 os.WriteFile("x",nil,0600); file.WriteString("x"); db.Exec("sql"); stmt,_:=db.Prepare("sql"); stmt.Exec()
 strconv.Atoi("1"); strconv.ParseInt("1",10,64); json.Unmarshal(nil,nil); dec.Decode(nil); xdec.Decode(nil); yaml.Unmarshal(nil,nil); toml.Unmarshal(nil,nil)
 time.Parse("",""); url.Parse(""); fmt.Sscanf("","%s",nil)
 exec.Command("echo"); Fake{}.Write(nil)
}
`})
	file := snapshot.Files["a.go"]
	if len(snapshot.Missing) > 0 {
		t.Fatal(snapshot.Missing)
	}
	counts := map[string]int{}
	for _, risk := range file.Risks {
		counts[risk.Kind]++
		if strings.Contains(risk.Evidence, "Fake") {
			t.Fatal(risk)
		}
		if risk.Declaration == "" {
			t.Fatal(risk)
		}
	}
	if counts["concurrency"] < 6 || counts["write"] != 4 || counts["input"] != 10 || counts["command"] < 1 {
		t.Fatal(counts, file.Risks)
	}
	if !file.Operations["risks"].Complete {
		t.Fatal(file.Operations)
	}
}

func TestExportedFunctionDeclarationIsNotDuplicated(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.ts": "export const value = () => 1;"})
	if len(snapshot.Missing) > 0 || len(snapshot.Files["a.ts"].Declarations) != 1 {
		t.Fatal(snapshot.Missing, snapshot.Files["a.ts"].Declarations)
	}
}

func TestGoEntryMethodsAliasesAndUncertainIndirectCalls(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"a.go": `package app
import("net/http";"flag";"os";"github.com/labstack/echo/v4";"github.com/urfave/cli";"github.com/gin-gonic/gin")
const cycle=cycle+cycle
func f(){
 http.HandleFunc("GET /csv",nil)
 echo.New().Match([]string{"GET","POST"},"/export",nil)
 _=&cli.StringFlag{Name:"format, f"}
 alias:=flag.String; alias("indirect","","")
 os.Getenv(cycle)
 g:=gin.New().Group("/v1"); if enabled(){g=gin.New().Group("/v2")};g.GET("/unknown",nil)
}
func enabled()bool{return true}
`})
	file := snapshot.Files["a.go"]
	want := []string{"route:GET /csv", "route:GET /export", "route:POST /export", "flag:--format", "flag:-f"}
	slices.Sort(want)
	if got := entryNames(file); !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for _, reason := range []string{"间接传递", "常量引用出现循环", "前缀"} {
		found := false
		for _, missing := range file.Missing {
			found = found || strings.Contains(missing.Reason, reason)
		}
		if !found {
			t.Fatalf("missing %s: %+v", reason, file.Missing)
		}
	}
}

func TestOperationResultsAndTopLevelReferencesRemainSeparate(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"a.py":       "def run():\n return 1\nrun()\ntext='unused'\n",
		"opaque.zzz": "unknown syntax",
		"a.go":       "package p\nfunc f(){}\n",
		"go.mod":     "module example.test/app\nrequire example.test/dep v1.0.0\n",
	})
	if len(snapshot.Missing) > 0 {
		t.Fatal(snapshot.Missing)
	}
	if !slices.Contains(snapshot.Files["a.py"].References, "run") || slices.Contains(snapshot.Files["a.py"].References, "unused") {
		t.Fatal(snapshot.Files["a.py"].References)
	}
	if !snapshot.Files["a.py"].Operations["declarations"].Complete || !snapshot.Files["a.py"].Operations["entries"].Complete || !snapshot.Files["a.py"].Operations["tests"].Complete {
		t.Fatal(snapshot.Files["a.py"].Operations)
	}
	if !snapshot.Files["opaque.zzz"].Operations["files"].Complete || snapshot.Files["opaque.zzz"].Operations["declarations"].Complete {
		t.Fatal(snapshot.Files["opaque.zzz"].Operations)
	}
	if !snapshot.Files["go.mod"].Operations["dependencies"].Complete || !snapshot.Files["a.go"].Operations["usage"].Complete || !snapshot.Files["a.go"].Operations["tests"].Complete {
		t.Fatal(snapshot.Files["a.go"].Operations)
	}
}

func TestEntryDependencyAndDeletedPositionAssociation(t *testing.T) {
	before := scanSources(t, map[string]string{"a.go": "package p\nimport \"os\"\nfunc run(){os.Getenv(\"OLD\")}\n"})
	after := scanSources(t, map[string]string{
		"a.go":   "package p\nimport \"flag\"\nfunc run(){flag.String(\"format\",\"\",\"\");helper()}\nfunc helper(){}\n",
		"go.mod": "module example.test/app\nrequire example.test/dep v1.0.0\n", "go.sum": "lock data",
	})
	changes, err := Compare(t.Context(), before, after, nil)
	if err != nil {
		t.Fatal(err)
	}
	association, err := Associate(t.Context(), before, after, changes, []Position{
		{Requirement: "R1", At: "flag:--format", Purpose: "选择输出格式"},
		{Requirement: "R2", At: "dep:example.test/dep", Purpose: "输出解析"},
		{Requirement: "R3", At: "env:OLD", Change: "delete", Purpose: "删除旧开关"},
		{Requirement: "R4", At: "env:missing", Purpose: "未能定位"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(association.Requirements["a.go:helper"], []string{"R1", "R3"}) || !slices.Equal(association.Requirements["go.sum"], []string{"R2"}) {
		t.Fatal(association)
	}
	if len(association.Pending) != 1 || association.Pending[0].Requirement != "R4" || len(association.Requirements["env:missing"]) != 0 {
		t.Fatal(association)
	}
}
