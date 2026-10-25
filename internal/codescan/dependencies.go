package codescan

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type Dependency struct {
	Manifest string
	Name     string
	Locks    []string
}

var packageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)

func Dependencies(ctx context.Context, filename string, data []byte) ([]Dependency, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var names, locks []string
	var err error
	switch base := path.Base(filename); base {
	case "go.mod":
		names, err = goDependencies(data)
		locks = []string{"go.sum"}
	case "package.json":
		names, err = jsonDependencies(data, []string{"dependencies", "devDependencies", "peerDependencies", "optionalDependencies"})
		locks = []string{"package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", "bun.lock"}
	case "composer.json":
		names, err = jsonDependencies(data, []string{"require", "require-dev"})
		locks = []string{"composer.lock"}
	case "pyproject.toml":
		names, err = tomlDependencies(ctx, data, false)
		locks = []string{"poetry.lock", "uv.lock", "pdm.lock"}
	case "Cargo.toml":
		names, err = tomlDependencies(ctx, data, true)
		locks = []string{"Cargo.lock"}
	case "pom.xml":
		names, err = xmlDependencies(data, false)
	case "build.gradle", "build.gradle.kts":
		names, err = scriptDependencies(data, false)
		locks = []string{"gradle.lockfile"}
	case "Gemfile":
		names, err = scriptDependencies(data, true)
		locks = []string{"Gemfile.lock"}
	default:
		if strings.HasSuffix(base, ".csproj") {
			names, err = xmlDependencies(data, true)
			locks = []string{"packages.lock.json"}
		} else if strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt") {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
					continue
				}
				if strings.Contains(strings.Fields(line)[0], "://") {
					err = errors.Join(err, fmt.Errorf("未注明包名的依赖地址无法确定名字"))
				} else if name := packageName.FindString(line); name != "" {
					names = append(names, name)
				}
			}
		}
	}
	for i, lock := range locks {
		locks[i] = path.Join(path.Dir(filename), lock)
	}
	sort.Strings(names)
	var result []Dependency
	for _, name := range names {
		if name != "" && (len(result) == 0 || result[len(result)-1].Name != name) {
			result = append(result, Dependency{Manifest: filename, Name: name, Locks: append([]string(nil), locks...)})
		}
	}
	if err != nil {
		return result, fmt.Errorf("%s 的直接依赖未完成解析: %w", filename, err)
	}
	return result, nil
}

func AddedDependencies(before, after []Dependency) []Dependency {
	seen := map[string]bool{}
	for _, dependency := range before {
		seen[dependency.Manifest+"\x00"+dependency.Name] = true
	}
	var added []Dependency
	for _, dependency := range after {
		if !seen[dependency.Manifest+"\x00"+dependency.Name] {
			added = append(added, dependency)
		}
	}
	return added
}

func jsonDependencies(data []byte, groups []string) ([]string, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	var names []string
	for _, group := range groups {
		if raw, ok := document[group]; ok {
			var items map[string]json.RawMessage
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, err
			}
			for name := range items {
				names = append(names, name)
			}
		}
	}
	return names, nil
}

func goDependencies(data []byte) ([]string, error) {
	var names []string
	block := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), max(4096, len(data)+1))
	for scanner.Scan() {
		code, comment, _ := strings.Cut(scanner.Text(), "//")
		fields := strings.Fields(code)
		if len(fields) == 0 {
			continue
		}
		if !block {
			if fields[0] != "require" {
				continue
			}
			fields = fields[1:]
			if len(fields) == 1 && fields[0] == "(" {
				block = true
				continue
			}
		} else if len(fields) == 1 && fields[0] == ")" {
			block = false
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("require 项需要模块路径和版本")
		}
		words := strings.Fields(comment)
		if len(words) > 0 && words[0] == "indirect" {
			continue
		}
		name := fields[0]
		if strings.HasPrefix(name, `"`) {
			var err error
			name, err = strconv.Unquote(name)
			if err != nil {
				return nil, err
			}
		}
		names = append(names, name)
	}
	if block {
		return nil, fmt.Errorf("require 块未结束")
	}
	return names, scanner.Err()
}

func xmlDependencies(data []byte, csproj bool) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var stack, names []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return names, nil
		}
		if err != nil {
			return nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if csproj && token.Name.Local == "PackageReference" {
				for _, attribute := range token.Attr {
					if attribute.Name.Local == "Include" {
						names = append(names, attribute.Value)
					}
				}
			}
			if !csproj && token.Name.Local == "dependency" && len(stack) > 0 && stack[len(stack)-1] == "dependencies" {
				blocked := false
				for _, part := range stack {
					blocked = blocked || part == "dependencyManagement" || part == "plugin"
				}
				var dependency struct {
					Group string `xml:"groupId"`
					Name  string `xml:"artifactId"`
				}
				if err := decoder.DecodeElement(&dependency, &token); err != nil {
					return nil, err
				}
				if !blocked && dependency.Group != "" && dependency.Name != "" {
					names = append(names, strings.TrimSpace(dependency.Group)+":"+strings.TrimSpace(dependency.Name))
				}
				continue
			}
			stack = append(stack, token.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func tomlDependencies(ctx context.Context, data []byte, cargo bool) ([]string, error) {
	entry := grammars.DetectLanguageByName("toml")
	tree, err := parse(ctx, entry, data)
	if tree != nil {
		defer tree.Release()
	}
	if err != nil {
		return nil, err
	}
	language := entry.Language()
	var names []string
	var visit func(*ts.Node, []string)
	visit = func(node *ts.Node, prefix []string) {
		if node == nil || ctx.Err() != nil {
			return
		}
		switch node.Type(language) {
		case "table", "table_array_element":
			prefix = tomlKey(node.NamedChild(0), language, data)
			if cargo && cargoDependencyKey(prefix) || !cargo && poetryDependencyKey(prefix) {
				if name := prefix[len(prefix)-1]; cargo || name != "python" {
					names = append(names, name)
				}
				return
			}
			for i := 1; i < node.NamedChildCount(); i++ {
				visit(node.NamedChild(i), prefix)
			}
			return
		case "pair":
			key := append(append([]string(nil), prefix...), tomlKey(node.NamedChild(0), language, data)...)
			value := node.NamedChild(1)
			if cargo {
				if cargoDependencyKey(key) {
					names = append(names, key[len(key)-1])
					return
				}
			} else {
				if pythonDependencyArray(key) {
					if value != nil && value.Type(language) == "array" {
						for i := 0; i < value.NamedChildCount(); i++ {
							item := value.NamedChild(i)
							if item.Type(language) == "string" {
								name := packageName.FindString(strings.TrimSpace(tomlText(item.Text(data))))
								names = append(names, name)
							}
						}
					}
					return
				}
				if poetryDependencyKey(key) {
					if name := key[len(key)-1]; name != "python" {
						names = append(names, name)
					}
					return
				}
			}
			if value != nil && value.Type(language) == "inline_table" {
				for i := 0; i < value.NamedChildCount(); i++ {
					visit(value.NamedChild(i), key)
				}
			}
			return
		}
		for i := 0; i < node.NamedChildCount(); i++ {
			visit(node.NamedChild(i), prefix)
		}
	}
	visit(tree.RootNode(), nil)
	return names, ctx.Err()
}

func cargoDependencyKey(key []string) bool {
	if len(key) == 2 {
		return key[0] == "dependencies" || key[0] == "dev-dependencies" || key[0] == "build-dependencies"
	}
	return len(key) == 4 && key[0] == "target" && key[2] == "dependencies" || len(key) == 3 && key[0] == "workspace" && key[1] == "dependencies"
}

func pythonDependencyArray(key []string) bool {
	return len(key) == 2 && key[0] == "project" && key[1] == "dependencies" ||
		len(key) == 3 && key[0] == "project" && key[1] == "optional-dependencies" ||
		len(key) == 2 && key[0] == "dependency-groups"
}

func poetryDependencyKey(key []string) bool {
	return len(key) >= 4 && key[0] == "tool" && key[1] == "poetry" && (key[len(key)-2] == "dependencies" || key[len(key)-2] == "dev-dependencies")
}

func tomlKey(node *ts.Node, language *ts.Language, data []byte) []string {
	if node == nil {
		return nil
	}
	if node.Type(language) != "dotted_key" {
		return []string{tomlText(node.Text(data))}
	}
	var parts []string
	for i := 0; i < node.NamedChildCount(); i++ {
		parts = append(parts, tomlKey(node.NamedChild(i), language, data)...)
	}
	return parts
}

func tomlText(value string) string {
	if strings.HasPrefix(value, `"`) {
		if decoded, err := strconv.Unquote(value); err == nil {
			return decoded
		}
	}
	return strings.Trim(value, "'\"")
}

type dependencyToken struct {
	text   string
	quoted bool
}

func scriptDependencies(data []byte, ruby bool) ([]string, error) {
	tokens := dependencyTokens(string(data), ruby)
	var names []string
	var unknown []string
	for i, token := range tokens {
		if token.quoted {
			continue
		}
		if ruby && token.text != "gem" {
			continue
		}
		if !ruby {
			switch token.text {
			case "implementation", "api", "compileOnly", "runtimeOnly", "testImplementation", "testApi", "testCompileOnly", "testRuntimeOnly":
			default:
				continue
			}
		}
		j := i + 1
		if j < len(tokens) && tokens[j].text == "(" {
			j++
		}
		if j >= len(tokens) {
			unknown = append(unknown, token.text)
			continue
		}
		if !ruby && (tokens[j].text == "platform" || tokens[j].text == "enforcedPlatform") && j+2 < len(tokens) && tokens[j+1].text == "(" {
			j += 2
		}
		if tokens[j].quoted {
			value := tokens[j].text
			if strings.ContainsAny(value, "$#{}") {
				unknown = append(unknown, token.text)
				continue
			}
			if ruby {
				names = append(names, value)
			} else if parts := strings.Split(value, ":"); len(parts) >= 2 {
				names = append(names, parts[0]+":"+parts[1])
			} else {
				unknown = append(unknown, token.text)
			}
		} else if !ruby && j+6 < len(tokens) && tokens[j].text == "group" && tokens[j+1].text == ":" && tokens[j+2].quoted && tokens[j+3].text == "," && tokens[j+4].text == "name" && tokens[j+5].text == ":" && tokens[j+6].quoted {
			names = append(names, tokens[j+2].text+":"+tokens[j+6].text)
		} else if tokens[j].text != "project" && tokens[j].text != "=" {
			unknown = append(unknown, token.text)
		}
	}
	if len(unknown) > 0 {
		return names, fmt.Errorf("依赖调用 %s 含不能静态确定名字的表达式", strings.Join(unknown, ", "))
	}
	return names, nil
}

/**
 * 只识别清单调用中的坐标字面量，不执行构建脚本；注释和字符串正文不能
 * 再被当作调用。动态表达式的名字需由后续检查保留为无法确定的范围。
 */
func dependencyTokens(source string, ruby bool) []dependencyToken {
	var tokens []dependencyToken
	for i := 0; i < len(source); {
		r, size := utf8.DecodeRuneInString(source[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		if ruby && source[i] == '#' || !ruby && strings.HasPrefix(source[i:], "//") {
			if end := strings.IndexByte(source[i:], '\n'); end >= 0 {
				i += end + 1
				continue
			}
			break
		}
		if !ruby && strings.HasPrefix(source[i:], "/*") {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				break
			}
			i += end + 4
			continue
		}
		if source[i] == '\'' || source[i] == '"' {
			quote, start := source[i], i+1
			i++
			for i < len(source) && source[i] != quote {
				if source[i] == '\\' {
					i++
				}
				i++
			}
			if i >= len(source) {
				break
			}
			tokens = append(tokens, dependencyToken{text: source[start:i], quoted: true})
			i++
			continue
		}
		start := i
		if unicode.IsLetter(r) || r == '_' {
			for i < len(source) {
				r, size = utf8.DecodeRuneInString(source[i:])
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
					break
				}
				i += size
			}
		} else {
			i += size
		}
		tokens = append(tokens, dependencyToken{text: source[start:i]})
	}
	return tokens
}
