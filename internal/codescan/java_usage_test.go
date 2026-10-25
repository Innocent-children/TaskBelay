package codescan

import (
	"slices"
	"strings"
	"testing"
)

func TestJavaAbstractionsErrorsVisibilityAndImplicitUses(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"lib/pom.xml":                   `<project><artifactId>lib</artifactId></project>`,
		"lib/src/main/java/Reader.java": `package demo; public interface Reader {String read(String input);}`,
		"lib/src/main/java/Base.java":   `package demo; public abstract class Base implements Reader {public abstract String read(String input);}`,
		"lib/src/main/java/Impl.java": `package demo;
public class Impl extends Base {
 @Mapped private boolean enabled;
 public String read(String input){return input;}
 public String toString(){return "impl";}
 @Mapped public void dispatch(int value){}
 public Reader anonymous(){return new Reader(){public String read(String input){return input;}};}
 /** @deprecated use read */ public String old(String input){return read(input);}
 @Deprecated public void legacy(){}
 // @deprecated ordinary comment
 public void ordinary(){}
 private void internal(int value){}
}
@interface Mapped {}
class Fault extends RuntimeException {}
class Specific extends Fault {}
class Other extends Exception {}
class Handling {void read(Throwable failure){try{}catch(Fault|Other e){} if(failure instanceof Specific){} }}
enum Mode {FIRST,SECOND;void read(int count){}}
record Pair(int count,String text){}
`,
		"lib/src/test/java/Fake.java": `package demo;class Fake implements Reader {public String read(String input){return input;}}`,
		"app/build.gradle":            `plugins { id 'java' }`,
		"app/src/main/java/App.java":  `package app;public class App {public static void main(String[] args){} public void read(int count){}}`,
		"boot/build.gradle.kts":       `plugins { java }`,
		"boot/App.java":               `import org.springframework.boot.autoconfigure.SpringBootApplication; @SpringBootApplication public class App {public void read(int count){}}`,
	})
	d := snapshot.Declarations()
	reader := d["lib/src/main/java/Reader.java:Reader"].Usage
	if reader == nil || !reader.Abstract || !reader.Public || !slices.Equal(reader.Methods, []string{"read"}) {
		t.Fatal(reader)
	}
	production, tests := 0, 0
	for _, use := range reader.Implementations {
		if use.Test {
			tests++
		} else {
			production++
		}
		if use.File == "" || use.Span.End <= use.Span.Start {
			t.Fatal(use)
		}
	}
	if production != 2 || tests != 1 {
		t.Fatal(reader.Implementations)
	}
	impl := d["lib/src/main/java/Impl.java:Impl"].Usage
	if !impl.Public || len(impl.Fields) != 1 || !impl.Fields[0].External {
		t.Fatal(impl)
	}
	for _, name := range []string{"read", "toString", "dispatch", "old", "legacy", "ordinary"} {
		u := d["lib/src/main/java/Impl.java:Impl."+name].Usage
		if !u.Public {
			t.Fatal(name, u)
		}
		if (name == "old" || name == "legacy") != u.Deprecated {
			t.Fatal(name, u)
		}
		if (name == "toString" || name == "dispatch" || name == "legacy") && len(u.Implicit) == 0 {
			t.Fatal(name, u)
		}
	}
	read := d["lib/src/main/java/Impl.java:Impl.read"].Usage
	if !read.Parameters[0].Interface || !read.Parameters[0].Public || !read.Parameters[0].Read {
		t.Fatal(read)
	}
	if d["lib/src/main/java/Impl.java:Impl.internal"].Usage.Public {
		t.Fatal("private exposed")
	}
	for _, name := range []string{"Fault", "Specific", "Other"} {
		u := d["lib/src/main/java/Impl.java:"+name].Usage
		if !u.Error || len(u.ErrorChecks) != 1 {
			t.Fatal(name, u)
		}
	}
	if d["lib/src/main/java/Impl.java:Mode.read"].Usage == nil || d["lib/src/main/java/Impl.java:Pair"].Usage == nil || d["lib/src/main/java/Impl.java:Mapped"].Usage == nil {
		t.Fatal(d)
	}
	if d["app/src/main/java/App.java:App"].Usage.Public || d["boot/App.java:App.read"].Usage.Public {
		t.Fatal("application module marked as library")
	}
	for _, file := range snapshot.Files {
		if file.Kind.Language == "java" && !file.CodeComplete {
			t.Fatal(file.Path, file.Missing)
		}
	}
}

func TestJavaGenericsExplicitDiamondAndMethodInference(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"Generic.java": `class Cache<T>{}
class Inferred<T>{}
class Raw<T>{}
class Calls {
 static <T>T explicit(T value){return value;}
 static <T>T inferred(T value){return value;}
 void run(){Cache<String> one=new Cache<java.lang.String>();Cache<Integer> two=new Cache<Integer>();Inferred<String> three=new Inferred<>();Raw raw=new Raw();Calls.<String>explicit("a");Calls.inferred("a");}
}`})
	d := snapshot.Declarations()
	cache := d["Generic.java:Cache"].Usage
	values := map[string]bool{}
	for _, use := range cache.TypeArguments {
		values[use.Value] = true
	}
	if !cache.Generic || cache.Inferred || len(values) != 2 || !values["<java.lang.String>"] || !values["<java.lang.Integer>"] {
		t.Fatal(cache)
	}
	for _, name := range []string{"Inferred", "Raw", "Calls.inferred"} {
		u := d["Generic.java:"+name].Usage
		if !u.Generic || !u.Inferred || len(u.Unknown) == 0 {
			t.Fatal(name, u)
		}
	}
	explicit := d["Generic.java:Calls.explicit"].Usage
	if explicit.Inferred || len(explicit.TypeArguments) != 1 || explicit.TypeArguments[0].Value != "<java.lang.String>" {
		t.Fatal(explicit)
	}
}

func TestJavaParameterFieldsConstructionAndValues(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"Values.java": `import java.util.function.Function;
class Values {
 boolean enabled;
 int count=1;
 long total;
 char marker;
 @Mapped String mapped;
 Function<String,String> callback;
 static final int DEFAULT=2;
 Values(int count,Function<String,String> callback){this.count=count;this.callback=callback;}
 int read(int value,boolean unused){value++;enabled=true;count+=value;return value;}
 void steady(long value){total=value;}
 void many(int... values){int[] copy=values;}
 static String transform(String text){return text;}
 void run(){Values first=new Values(DEFAULT,x->x);Values second=new Values(3,Values::transform);first.enabled=false;first.read(4,true);first.steady(0);first.many(1,2);first.many(1,2,2);first.many(new int[]{1,2});first.many();}
}
@interface Mapped {}
record Pair(int count,String text){}
class RecordCall {void run(){new Pair(4,"x");}}
`})
	d := snapshot.Declarations()
	u := d["Values.java:Values"].Usage
	fields := map[string]Slot{}
	for _, field := range u.Fields {
		fields[field.Name] = field
	}
	if !fields["enabled"].Written || len(fields["enabled"].Values) != 3 || !fields["enabled"].Values[0].Implicit {
		t.Fatal(fields["enabled"])
	}
	if len(fields["count"].Values) != 4 || len(fields["count"].Unknown) == 0 {
		t.Fatal(fields["count"])
	}
	if !fields["mapped"].External || fields["marker"].Values[0].Kind != "char" || fields["marker"].Values[0].Text != "\x00" {
		t.Fatal(fields)
	}
	if !fields["callback"].Function || len(fields["callback"].Functions) != 2 {
		t.Fatal(fields["callback"])
	}
	if len(fields["total"].Values) != 2 {
		t.Fatal(fields["total"])
	}
	for _, v := range fields["total"].Values {
		if v.Kind != "long" || v.Text != "0" {
			t.Fatal(v)
		}
	}
	read := d["Values.java:Values.read"].Usage
	if !read.Parameters[0].Read || !read.Parameters[0].Written || len(read.Parameters[0].Unknown) == 0 || read.Parameters[1].Read || len(read.Parameters[1].Values) != 1 {
		t.Fatal(read)
	}
	many := d["Values.java:Values.many"].Usage.Parameters[0]
	sequences := map[string]bool{}
	for _, value := range many.Values {
		sequences[value.Text] = true
	}
	if !many.Variadic || !many.Read || len(many.Unknown) > 0 || len(many.Values) != 4 || len(sequences) != 3 {
		t.Fatal(many)
	}
	for _, field := range d["Values.java:Pair"].Usage.Fields {
		if len(field.Values) != 2 || !field.Written {
			t.Fatal(field)
		}
	}
	if len(d["Values.java:Values.transform"].Usage.Parameters[0].Unknown) == 0 {
		t.Fatal("indirect call treated as complete")
	}
}

func TestJavaShadowedParametersOverloadsAndIndirectWrites(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"Unknown.java": `import java.util.function.Function;
class State {
 int count;
 State(int initial){count=initial;}
 void use(int value) {Function<String,String> f=new Function<>(){public String apply(String value){return value;}};}
 void overloaded(int value){}
 void overloaded(String value){}
 void escape(State other){External.save(other);}
 void run(){State state=new State(1);state.overloaded(2);state.count++;escape(state);}
}`})
	d := snapshot.Declarations()
	if d["Unknown.java:State.use"].Usage.Parameters[0].Read {
		t.Fatal(d["Unknown.java:State.use"].Usage)
	}
	count := 0
	for _, item := range d {
		if item.Name == "overloaded" {
			count++
			if len(item.Usage.Parameters[0].Unknown) == 0 || len(item.Usage.Parameters[0].Values) > 0 {
				t.Fatal(item.Usage)
			}
		}
	}
	if count != 2 {
		t.Fatal(d)
	}
	field := d["Unknown.java:State"].Usage.Fields[0]
	reasons := []string{}
	for _, u := range field.Unknown {
		reasons = append(reasons, u.Reason)
	}
	if !strings.Contains(strings.Join(reasons, "\n"), "间接写入") || !strings.Contains(strings.Join(reasons, "\n"), "自增减") || snapshot.Files["Unknown.java"].Operations["usage"].Complete {
		t.Fatal(field)
	}
}

func TestJavaPrimitiveLiteralNormalization(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"Numbers.java": `class Numbers {
 void read(int a,long b,float c,double d,char e,boolean f,Object g){}
 void run(){read(0xFFFFFFFF,2L,1.0f,-0.0,'x',false,null);read(-2147483648,0xFFFFFFFFFFFFFFFFL,2f,1e2,'\u0061',true,null);}
}`})
	u := snapshot.Declarations()["Numbers.java:Numbers.read"].Usage
	want := [][]string{{"-1", "-2147483648"}, {"2", "-1"}, {"1", "2"}, {"-0", "100"}, {"x", "a"}, {"false", "true"}, {"null", "null"}}
	for i, slot := range u.Parameters {
		if len(slot.Unknown) > 0 || len(slot.Values) != 2 {
			t.Fatal(slot)
		}
		for j, v := range slot.Values {
			if v.Text != want[i][j] {
				t.Fatal(i, j, v, want[i])
			}
		}
	}
}

func TestJavaInheritedCallsCompactRecordsEnumsAndLocalTypes(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"a/Base.java": `package a;
interface Reader {int read(int value);}
class Base {int count;Base(int initial){count=initial;} int read(int value){return value;} }
class Child extends Base {Child(int initial){super(initial);} int read(int value){return value+1;} int parent(){return super.read(9);} }
record Pair(int count,String text){Pair{count++;}}
enum Mode{FIRST,SECOND;int count;void write(int value){count=value;}}
class Calls {
 void call(Base value){value.read(1);}
 void run(){new Child(2);new Pair(4,"x");Mode.FIRST.write(3);}
 void local(){class System{String getenv(String key){return key;}}System value=new System();value.getenv("LOCAL");class ReaderImpl implements Reader{public int read(int value){return value;}}}
 String real(){return System.getenv("REAL");}
 void another(){class System{String getenv(String key){return key;}}new System().getenv("SECOND");}
}`,
		"Config.java": `import org.springframework.boot.context.properties.ConfigurationProperties;
@ConfigurationProperties(prefix="http") record Config(int port,String host) {}`,
	})
	d := snapshot.Declarations()
	for _, name := range []string{"Base.read", "Child.read"} {
		u := d["a/Base.java:"+name].Usage
		if len(u.Parameters[0].Unknown) == 0 {
			t.Fatal(name, u)
		}
	}
	base := d["a/Base.java:Base.read"].Usage
	if len(base.Parameters[0].Values) != 1 || base.Parameters[0].Values[0].Text != "9" {
		t.Fatal(base)
	}
	ctor := d["a/Base.java:Pair.Pair"].Usage
	if ctor == nil || len(ctor.Parameters) != 2 || !ctor.Parameters[0].Written || len(ctor.Parameters[0].Unknown) == 0 || ctor.Parameters[0].Values[0].Text != "4" {
		t.Fatal(ctor)
	}
	pair := d["a/Base.java:Pair"].Usage
	if len(pair.Fields) != 2 || !pair.Fields[0].Written || len(pair.Fields[0].Unknown) == 0 {
		t.Fatal(pair)
	}
	mode := d["a/Base.java:Mode"].Usage
	if len(mode.Fields) != 1 || !mode.Fields[0].Written || len(mode.Fields[0].Values) != 2 || mode.Fields[0].Values[1].Text != "3" {
		t.Fatal(mode)
	}
	reader := d["a/Base.java:Reader"].Usage
	if len(reader.Implementations) != 1 || !strings.Contains(reader.Implementations[0].Value, ":local@") {
		t.Fatal(reader)
	}
	var env []string
	for _, entry := range snapshot.Files["a/Base.java"].Entries {
		if entry.Kind == "env" {
			env = append(env, entry.Name)
		}
	}
	if !slices.Equal(env, []string{"REAL"}) {
		t.Fatal(env, snapshot.Files["a/Base.java"].Missing)
	}
	var config []string
	for _, entry := range snapshot.Files["Config.java"].Entries {
		config = append(config, entry.Location())
	}
	if !slices.Contains(config, "cfg:http.port") || !slices.Contains(config, "cfg:http.host") {
		t.Fatal(config)
	}
	for _, file := range snapshot.Files {
		if !file.CodeComplete {
			t.Fatal(file.Path, file.Missing)
		}
	}
}

func TestJavaCompleteUsageAndCrossFileSources(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"demo/State.java": `package demo;class State{boolean enabled;int count;State(int value){count=value;}int read(int input){return count+input;}void update(){enabled=true;}}`,
		"demo/Main.java":  `package demo;class Main {void run(){State first=new State(1);first.read(2);first.update();}}`,
	})
	for _, file := range snapshot.Files {
		if !file.Operations["usage"].Complete {
			t.Fatal(file.Path, file.Missing)
		}
	}
	d := snapshot.Declarations()
	count := d["demo/State.java:State"].Usage.Fields[1]
	if !count.Read || !count.Written || len(count.Values) != 2 || count.Values[1].At.File != "demo/Main.java" {
		t.Fatal(count)
	}
	input := d["demo/State.java:State.read"].Usage.Parameters[0]
	if !input.Read || len(input.Values) != 1 || input.Values[0].At.File != "demo/Main.java" {
		t.Fatal(input)
	}
}

func TestJavaFunctionContractsArrayWritesAndQualifiedThis(t *testing.T) {
	snapshot := scanSources(t, map[string]string{"Cases.java": `interface Callback {int apply(int value);default void prepare(){}}
class Outer {
 int value;
 int[] items;
 void accept(Callback fn){fn.apply(1);fn.prepare();}
 void mutate(int[] values){values[0]++;items[0]=1;}
 class Inner {void change(){Outer.this.value=2;}}
 void run(){accept(x->x);}
}
public record PublicRecord(int value){}
`})
	d := snapshot.Declarations()
	fn := d["Cases.java:Outer.accept"].Usage.Parameters[0]
	if !fn.Function || !fn.Read || len(fn.Functions) != 1 {
		t.Fatal(fn)
	}
	parameter := d["Cases.java:Outer.mutate"].Usage.Parameters[0]
	if !parameter.Written || !parameter.Read || len(parameter.Unknown) == 0 {
		t.Fatal(parameter)
	}
	u := d["Cases.java:Outer"].Usage
	if len(u.Fields[0].Values) != 2 || u.Fields[0].Values[1].Text != "2" || !u.Fields[1].Written || len(u.Fields[1].Unknown) == 0 {
		t.Fatal(u)
	}
	if len(d["Cases.java:PublicRecord"].Usage.Fields[0].Unknown) == 0 {
		t.Fatal("public record external constructor ignored")
	}
}
