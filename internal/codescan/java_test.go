package codescan

import (
	"slices"
	"strings"
	"testing"
)

func TestJavaDocumentedAnnotationAndEnvironmentEntries(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"src/main/java/demo/Names.java": `package demo; public class Names {public static final String ROOT="/v1";public static final String ENV="APP_"+"PORT";}`,
		"src/main/java/demo/Server.java": `package demo;
import picocli.CommandLine.Option;
import picocli.CommandLine.Command;
import org.springframework.web.bind.annotation.*;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.context.properties.ConfigurationProperties;
import static demo.Names.ENV;
import static java.lang.System.getenv;
@Command(name="serve")
@ConfigurationProperties(prefix="server")
@RequestMapping(path={Names.ROOT,"/v2"})
class Server {
 @Option(names={"--port","-p"}) int port=1;
 @Value("${server.timeout:10}-${mode}") String timeout;
 static final String IGNORED="internal";
 @GetMapping(path="/items") String list(){return System.getenv(ENV);}
 @RequestMapping(value="/status",method={RequestMethod.GET,RequestMethod.POST}) String status(){return getenv("STATE");}
 @PostMapping("/create") void create(){}
 @PutMapping("/replace") void replace(){}
 @PatchMapping("/change") void change(){}
 @DeleteMapping("/remove") void remove(){}
}
`,
		"src/main/java/demo/Jax.java": `package demo; import jakarta.ws.rs.Path; @Path("/service") class Jax { @Path("/item") public String get(){return "ok";} }`,
	})
	entries := map[string]bool{}
	for _, file := range snapshot.Files {
		for _, entry := range file.Entries {
			entries[entry.Kind+":"+entry.Name+" "+entry.Method] = true
			if entry.Declaration == "" || entry.Evidence == "" {
				t.Fatal(entry)
			}
		}
		if file.Kind.Language == "java" && !file.Operations["entries"].Complete {
			t.Fatal(file.Missing)
		}
	}
	for _, want := range []string{"flag:--port ", "flag:-p ", "cmd:serve ", "env:APP_PORT ", "env:STATE ", "route:/v1/items GET", "route:/v2/items GET", "route:/v1/status GET", "route:/v1/status POST", "route:/v2/status POST", "route:/service/item ", "cfg:server.port ", "cfg:server.timeout ", "cfg:mode "} {
		if !entries[want] {
			t.Fatal(want, entries, snapshot.Missing)
		}
	}
	if entries["cfg:server.IGNORED "] {
		t.Fatal(entries)
	}
	for _, want := range []string{"route:/v1/create POST", "route:/v2/replace PUT", "route:/v1/change PATCH", "route:/v2/remove DELETE"} {
		if !entries[want] {
			t.Fatal(want, entries)
		}
	}
}

func TestJavaStaticImportsInheritedAPIsAndMethodReferences(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"Names.java": `package sample; public interface Names {String ROOT="/api";String KEY="KEY";}`,
		"Service.java": `package sample;
import static java.lang.System.*;
import static org.springframework.web.bind.annotation.RequestMethod.GET;
import static picocli.CommandLine.Option;
import org.springframework.web.bind.annotation.RequestMapping;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.nio.file.Files;
class Mapper extends ObjectMapper {}
class Base implements Names {}
class Service extends Base {
 @Option(names="--value") String value;
 @RequestMapping(path=ROOT,method=GET) void serve(){getenv(KEY);new Mapper().readValue("{}",Object.class);}
 void references(){var env=System::getenv;var file=Files::writeString;}
}
`,
	})
	file := snapshot.Files["Service.java"]
	for _, source := range snapshot.Files {
		if !source.CodeComplete {
			t.Fatal(source.Path, source.Missing)
		}
	}
	var entries []string
	for _, entry := range file.Entries {
		entries = append(entries, entry.Location())
	}
	for _, want := range []string{"flag:--value", "env:KEY", "route:GET /api"} {
		if !slices.Contains(entries, want) {
			t.Fatal(want, entries, file.Missing)
		}
	}
	input, write := false, false
	for _, risk := range file.Risks {
		input = input || risk.Kind == "input"
		write = write || risk.Kind == "write"
	}
	if !input || !write || file.Operations["entries"].Complete || !file.Operations["risks"].Complete {
		t.Fatal(file.Risks, file.Missing, file.Operations)
	}
}

func TestJavaScopesAmbiguousImportsAndReflectionRemainUnknown(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"Scoped.java": `import org.springframework.web.bind.annotation.GetMapping;
class Scoped {
 @interface GetMapping {String value();}
 @GetMapping("/not-framework") void route(){}
 void local(){ {Object System=null;System.getenv("NO");}System.getenv("YES"); }
 void reflection()throws Exception{Class.forName("Target").getMethod("action").invoke(null);}
}`,
		"Ambiguous.java": `import external.unknown.*;class Ambiguous {void run(){System.getenv("NOT_CONFIRMED");}}`,
		"One.java":       `package duplicate; class Names{static final String KEY="ONE";}`,
		"Two.java":       `package duplicate; class Names{static final String KEY="TWO";}`,
		"Use.java":       `package duplicate;class Use{void run(){System.getenv(Names.KEY);}}`,
	})
	file := snapshot.Files["Scoped.java"]
	if len(file.Entries) != 1 || file.Entries[0].Name != "YES" || file.Operations["entries"].Complete || file.Operations["risks"].Complete {
		t.Fatal(file.Entries, file.Missing)
	}
	for _, name := range []string{"Ambiguous.java", "Use.java"} {
		file := snapshot.Files[name]
		if len(file.Entries) != 0 || file.Operations["entries"].Complete {
			t.Fatal(name, file.Entries, file.Missing)
		}
	}
}

func TestJavaInheritedMappingsOverloadsAndEmptyPaths(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"Boundaries.java": `import org.springframework.web.bind.annotation.*;
import com.fasterxml.jackson.databind.ObjectMapper;
@RequestMapping("/base") class Base {}
class Child extends Base {@GetMapping("/child") void read(){}}
class Mapper extends ObjectMapper {String readValue(String text){return text;}}
@RequestMapping("/own") class Own {@GetMapping("") void root(){} }
class Plain {@GetMapping(path={}) void root(){}}
class Calls {void read()throws Exception{new Mapper().readValue("{}",Object.class);Calls.class.getMethod("action").invoke(null);}}
`})
	file := snapshot.Files["Boundaries.java"]
	var names []string
	for _, entry := range file.Entries {
		names = append(names, entry.Location())
	}
	if !slices.Contains(names, "route:GET /own") || slices.Contains(names, "route:GET /child") || slices.Contains(names, "route:GET /own/") || file.Operations["entries"].Complete || file.Operations["risks"].Complete {
		t.Fatal(names, file.Missing)
	}
	var reasons []string
	for _, m := range file.Missing {
		reasons = append(reasons, m.Reason)
	}
	for _, want := range []string{"继承路径", "重载", "反射调用", "路由路径"} {
		if !strings.Contains(strings.Join(reasons, "\n"), want) {
			t.Fatal(want, reasons)
		}
	}
}

func TestJavaDocumentedRisksAndTypedReceivers(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"Service.java": `import java.nio.file.Files;
import java.nio.file.Paths;
import java.io.FileWriter;
import java.io.FileOutputStream;
import java.sql.*;
import java.time.LocalDate;
import java.time.LocalDateTime;
import java.net.URI;
import com.fasterxml.jackson.databind.ObjectMapper;
import javax.persistence.EntityManager;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.CompletableFuture;
class Service {
 void run(Statement statement, PreparedStatement prepared, CallableStatement callable, EntityManager entities, ObjectMapper mapper, ExecutorService executor) throws Exception {
  new Thread(()->{}).start();executor.submit(()->{});CompletableFuture.runAsync(()->{});
  synchronized(this){}
  Files.write(Paths.get("x"),new byte[0]);Files.writeString(Paths.get("x"),"x");Files.delete(Paths.get("x"));Files.deleteIfExists(Paths.get("x"));Files.move(Paths.get("a"),Paths.get("b"));Files.copy(Paths.get("a"),Paths.get("b"));Files.createDirectories(Paths.get("x"));
  new FileWriter("x");new FileOutputStream("x");statement.execute("sql");statement.executeUpdate("sql");prepared.execute();callable.executeUpdate();entities.persist(new Object());entities.merge(new Object());entities.remove(new Object());
  DriverManager.getConnection("db").prepareStatement("sql").execute();
  Integer.parseInt("1");Long.parseLong("1");Double.parseDouble("1");mapper.readValue("{}",Object.class);mapper.readerFor(Object.class).readValue("{}");LocalDate.parse("2020-01-01");LocalDateTime.parse("2020-01-01T00:00");URI.create("/");
  new ProcessBuilder("command").start();Runtime.getRuntime().exec("command");
 }
 synchronized void lock(){}
}
`})
	file := snapshot.Files["Service.java"]
	var kinds []string
	for _, risk := range file.Risks {
		kinds = append(kinds, risk.Kind)
	}
	for _, kind := range []string{"concurrency", "write", "input", "command"} {
		if !slices.Contains(kinds, kind) {
			t.Fatal(kind, file.Risks, file.Missing)
		}
	}
	for _, expression := range []string{"statement.execute(\"sql\")", "prepared.execute()", "callable.executeUpdate()", "entities.merge(new Object())", "mapper.readerFor(Object.class).readValue(\"{}\")", "DriverManager.getConnection(\"db\").prepareStatement(\"sql\").execute()", "Runtime.getRuntime().exec(\"command\")"} {
		found := false
		for _, risk := range file.Risks {
			found = found || risk.Evidence == expression
		}
		if !found {
			t.Fatal(expression, file.Risks, file.Missing)
		}
	}
	if !file.Operations["risks"].Complete {
		t.Fatal(file.Missing)
	}
}

func TestJavaShadowingAndUnresolvedEntryNames(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"Example.java": `import picocli.CommandLine.Option;
import picocli.CommandLine.Command;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.beans.factory.annotation.Value;
class Fake {String getenv(String name){return name;} void writeString(String value){} }
class Example {
 @Option(names=dynamic) String option;
 @Command void command(){}
 @GetMapping void mapping(){}
 @Value("${prefix.${part}}") String value;
 void run(Fake System,Fake Files,Object statement){System.getenv("NOT_AN_ENV");Files.writeString("not a file");statement.execute("sql");}
 void other(String name){java.lang.System.getenv(name);}
}
`})
	file := snapshot.Files["Example.java"]
	if len(file.Entries) != 0 || len(file.Risks) != 0 || file.Operations["entries"].Complete || file.Operations["risks"].Complete || !file.CodeComplete {
		t.Fatal(file.Entries, file.Risks, file.Operations, file.Missing)
	}
	var reasons []string
	for _, m := range file.Missing {
		reasons = append(reasons, m.Reason)
	}
	if !strings.Contains(strings.Join(reasons, "\n"), "execute") {
		t.Fatal(reasons)
	}
}
