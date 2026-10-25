package golang

import (
	"github.com/Innocent-children/taskbelay/internal/lang"
	"strings"
)

var Rules = rules()

func rules() lang.Rules {
	r := lang.Rules{
		ImplicitNames:      []string{"main", "init"},
		ImplicitMethods:    []string{"String", "GoString", "Format", "Error", "Unwrap", "Is", "As", "MarshalJSON", "UnmarshalJSON", "MarshalText", "UnmarshalText", "ServeHTTP", "Read", "Write", "Close", "Len", "Less", "Swap", "Scan", "Value"},
		ImplicitDirectives: []string{"//go:linkname", "//export"},
		DeprecatedMarkers:  []string{"// Deprecated:"},
		ErrorConstructors:  []string{"errors.New", "fmt.Errorf"},
		ErrorChecks:        []string{"errors.Is", "errors.As"},
		ImportNames:        map[string]string{"github.com/pelletier/go-toml": "toml", "github.com/pelletier/go-toml/v2": "toml"},
		Constructors: map[string]string{
			"github.com/golang/mock/gomock.NewController": "github.com/golang/mock/gomock.Controller", "go.uber.org/mock/gomock.NewController": "go.uber.org/mock/gomock.Controller",
			"github.com/stretchr/testify/assert.New": "github.com/stretchr/testify/assert.Assertions", "github.com/stretchr/testify/require.New": "github.com/stretchr/testify/require.Assertions",
			"flag.NewFlagSet": "flag.FlagSet", "github.com/spf13/pflag.NewFlagSet": "github.com/spf13/pflag.FlagSet",
			"net/http.NewServeMux": "net/http.ServeMux", "os.Create": "os.File", "os.Open": "os.File", "os.OpenFile": "os.File",
			"database/sql.Open": "database/sql.DB", "database/sql.OpenDB": "database/sql.DB",
			"encoding/json.NewDecoder": "encoding/json.Decoder", "encoding/xml.NewDecoder": "encoding/xml.Decoder",
			"github.com/go-chi/chi.NewRouter": "github.com/go-chi/chi.Router", "github.com/go-chi/chi/v5.NewRouter": "github.com/go-chi/chi/v5.Router",
			"github.com/gin-gonic/gin.New": "github.com/gin-gonic/gin.Engine", "github.com/gin-gonic/gin.Default": "github.com/gin-gonic/gin.Engine",
			"github.com/labstack/echo.New": "github.com/labstack/echo.Echo", "github.com/labstack/echo/v4.New": "github.com/labstack/echo/v4.Echo",
		},
		ReceiverMethods: map[string]string{
			"github.com/spf13/cobra.Command.Flags": "github.com/spf13/pflag.FlagSet", "github.com/spf13/cobra.Command.PersistentFlags": "github.com/spf13/pflag.FlagSet",
			"database/sql.DB.Begin": "database/sql.Tx", "database/sql.DB.BeginTx": "database/sql.Tx",
			"database/sql.DB.Prepare": "database/sql.Stmt", "database/sql.DB.PrepareContext": "database/sql.Stmt",
			"database/sql.DB.Conn": "database/sql.Conn", "database/sql.Conn.BeginTx": "database/sql.Tx",
			"database/sql.Conn.PrepareContext": "database/sql.Stmt", "database/sql.Tx.Prepare": "database/sql.Stmt", "database/sql.Tx.PrepareContext": "database/sql.Stmt",
			"github.com/go-chi/chi.Router.With": "github.com/go-chi/chi.Router", "github.com/go-chi/chi/v5.Router.With": "github.com/go-chi/chi/v5.Router",
			"github.com/gin-gonic/gin.Engine.Group": "github.com/gin-gonic/gin.RouterGroup", "github.com/gin-gonic/gin.RouterGroup.Group": "github.com/gin-gonic/gin.RouterGroup",
			"github.com/labstack/echo.Echo.Group": "github.com/labstack/echo.Group", "github.com/labstack/echo/v4.Echo.Group": "github.com/labstack/echo/v4.Group",
		},
		FlagTypes:         []string{"github.com/urfave/cli.BoolFlag", "github.com/urfave/cli.StringFlag", "github.com/urfave/cli.IntFlag", "github.com/urfave/cli.Int64Flag", "github.com/urfave/cli.UintFlag", "github.com/urfave/cli.Uint64Flag", "github.com/urfave/cli.Float64Flag", "github.com/urfave/cli.DurationFlag", "github.com/urfave/cli.StringSliceFlag", "github.com/urfave/cli.IntSliceFlag", "github.com/urfave/cli.Int64SliceFlag", "github.com/urfave/cli.Float64SliceFlag", "github.com/urfave/cli.GenericFlag", "github.com/urfave/cli.PathFlag", "github.com/urfave/cli.TimestampFlag"},
		CommandTypes:      []string{"github.com/spf13/cobra.Command", "github.com/urfave/cli.Command", "github.com/urfave/cli/v2.Command", "github.com/urfave/cli/v3.Command"},
		ConfigTags:        []string{"yaml", "toml", "mapstructure"},
		ConcurrentNodes:   []string{"go_statement", "send_statement", "receive_statement", "select_statement"},
		ConcurrentImports: []string{"sync", "sync/atomic"},
	}
	for _, target := range []string{"os.Getenv", "os.LookupEnv"} {
		r.Entries = append(r.Entries, lang.CallRule{Target: target, Kind: "env"})
	}
	for _, prefix := range []string{"flag", "flag.FlagSet", "github.com/spf13/pflag", "github.com/spf13/pflag.FlagSet"} {
		for _, name := range []string{"Bool", "Duration", "Float64", "Int", "Int64", "String", "Uint", "Uint64"} {
			r.Entries = append(r.Entries, lang.CallRule{Target: prefix + "." + name, Kind: "flag"}, lang.CallRule{Target: prefix + "." + name + "Var", Kind: "flag", Arg: 1})
		}
		r.Entries = append(r.Entries, lang.CallRule{Target: prefix + ".Var", Kind: "flag", Arg: 1})
		if prefix == "flag" || prefix == "flag.FlagSet" {
			r.Entries = append(r.Entries, lang.CallRule{Target: prefix + ".Func", Kind: "flag"}, lang.CallRule{Target: prefix + ".BoolFunc", Kind: "flag"}, lang.CallRule{Target: prefix + ".TextVar", Kind: "flag", Arg: 1})
		}
		if prefix == "github.com/spf13/pflag" || prefix == "github.com/spf13/pflag.FlagSet" {
			for _, name := range []string{"Bool", "BytesBase64", "BytesHex", "Count", "Duration", "DurationSlice", "Float32", "Float32Slice", "Float64", "Float64Slice", "IP", "IPMask", "IPNet", "IPSlice", "Int", "Int8", "Int16", "Int32", "Int64", "IntSlice", "Int32Slice", "Int64Slice", "String", "StringArray", "StringSlice", "StringToInt", "StringToInt64", "StringToString", "Uint", "Uint8", "Uint16", "Uint32", "Uint64", "UintSlice"} {
				for _, suffix := range []string{"", "P"} {
					r.Entries = append(r.Entries, lang.CallRule{Target: prefix + "." + name + suffix, Kind: "flag"}, lang.CallRule{Target: prefix + "." + name + "Var" + suffix, Kind: "flag", Arg: 1})
				}
			}
			r.Entries = append(r.Entries, lang.CallRule{Target: prefix + ".VarP", Kind: "flag", Arg: 1})
		}
	}
	for _, prefix := range []string{"net/http", "net/http.ServeMux"} {
		for _, name := range []string{"Handle", "HandleFunc"} {
			r.Entries = append(r.Entries, lang.CallRule{Target: prefix + "." + name, Kind: "route"})
		}
	}
	for _, prefix := range []string{"github.com/go-chi/chi.Router", "github.com/go-chi/chi/v5.Router"} {
		for _, name := range []string{"Connect", "Delete", "Get", "Head", "Options", "Patch", "Post", "Put", "Trace", "Handle", "HandleFunc", "Mount", "Route"} {
			r.Entries = append(r.Entries, lang.CallRule{Target: prefix + "." + name, Kind: "route"})
		}
		for _, name := range []string{"Method", "MethodFunc"} {
			r.Entries = append(r.Entries, lang.CallRule{Target: prefix + "." + name, Kind: "route", Arg: 1})
		}
	}
	for _, prefix := range []string{"github.com/gin-gonic/gin.Engine", "github.com/gin-gonic/gin.RouterGroup", "github.com/gin-gonic/gin.IRouter", "github.com/gin-gonic/gin.IRoutes", "github.com/labstack/echo.Echo", "github.com/labstack/echo.Group", "github.com/labstack/echo/v4.Echo", "github.com/labstack/echo/v4.Group"} {
		for _, name := range []string{"CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT", "TRACE", "Any", "Static", "StaticFS", "File"} {
			r.Entries = append(r.Entries, lang.CallRule{Target: prefix + "." + name, Kind: "route"})
		}
		if strings.HasPrefix(prefix, "github.com/gin-gonic/gin.") {
			r.Entries = append(r.Entries, lang.CallRule{Target: prefix + ".Handle", Kind: "route", Arg: 1})
		} else {
			r.Entries = append(r.Entries, lang.CallRule{Target: prefix + ".Add", Kind: "route", Arg: 1}, lang.CallRule{Target: prefix + ".Match", Kind: "route", Arg: 1})
		}
	}
	for _, prefix := range []string{"github.com/urfave/cli/v2", "github.com/urfave/cli/v3"} {
		for _, name := range append([]string(nil), r.FlagTypes...) {
			if len(name) > len("github.com/urfave/cli.") && name[:len("github.com/urfave/cli.")] == "github.com/urfave/cli." {
				r.FlagTypes = append(r.FlagTypes, prefix+name[len("github.com/urfave/cli"):])
			}
		}
	}
	add := func(kind, prefix string, names ...string) {
		for _, name := range names {
			r.Risks = append(r.Risks, lang.CallRule{Target: prefix + "." + name, Kind: kind})
		}
	}
	add("write", "os", "WriteFile", "Create", "OpenFile", "Remove", "RemoveAll", "Rename", "Mkdir", "MkdirAll")
	add("write", "os.File", "Write", "WriteAt", "WriteString")
	for _, prefix := range []string{"database/sql.DB", "database/sql.Tx", "database/sql.Conn", "database/sql.Stmt"} {
		add("write", prefix, "Exec", "ExecContext")
	}
	add("input", "strconv", "Parse*", "Atoi")
	add("input", "encoding/json", "Unmarshal")
	add("input", "encoding/json.Decoder", "Decode")
	add("input", "encoding/xml", "Unmarshal")
	add("input", "encoding/xml.Decoder", "Decode")
	for _, prefix := range []string{"gopkg.in/yaml.v2", "gopkg.in/yaml.v3", "go.yaml.in/yaml/v3", "sigs.k8s.io/yaml", "github.com/BurntSushi/toml", "github.com/pelletier/go-toml", "github.com/pelletier/go-toml/v2"} {
		add("input", prefix, "Unmarshal")
	}
	add("input", "time", "Parse")
	add("input", "net/url", "Parse")
	add("input", "fmt", "Sscan*")
	add("command", "os/exec", "*")
	return r
}
