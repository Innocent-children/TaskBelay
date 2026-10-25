package java

import (
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang"
)

var Rules = lang.Rules{
	ImplicitMethods: []string{"main", "toString", "equals", "hashCode", "compareTo", "close", "run", "call"},
	Entries:         []lang.CallRule{{Target: "java.lang.System.getenv", Kind: "env"}},
	Risks: []lang.CallRule{
		{Target: "java.lang.Thread.*", Kind: "concurrency"},
		{Target: "java.util.concurrent.*", Kind: "concurrency"},
		{Target: "java.nio.file.Files.write", Kind: "write"}, {Target: "java.nio.file.Files.writeString", Kind: "write"},
		{Target: "java.nio.file.Files.delete", Kind: "write"}, {Target: "java.nio.file.Files.deleteIfExists", Kind: "write"},
		{Target: "java.nio.file.Files.move", Kind: "write"}, {Target: "java.nio.file.Files.copy", Kind: "write"}, {Target: "java.nio.file.Files.createDirectories", Kind: "write"},
		{Target: "java.io.FileOutputStream.*", Kind: "write"}, {Target: "java.io.FileWriter.*", Kind: "write"},
		{Target: "java.sql.Statement.execute", Kind: "write"}, {Target: "java.sql.Statement.executeUpdate", Kind: "write"},
		{Target: "java.sql.PreparedStatement.execute", Kind: "write"}, {Target: "java.sql.PreparedStatement.executeUpdate", Kind: "write"},
		{Target: "java.sql.CallableStatement.execute", Kind: "write"}, {Target: "java.sql.CallableStatement.executeUpdate", Kind: "write"},
		{Target: "javax.persistence.EntityManager.persist", Kind: "write"}, {Target: "javax.persistence.EntityManager.merge", Kind: "write"}, {Target: "javax.persistence.EntityManager.remove", Kind: "write"},
		{Target: "jakarta.persistence.EntityManager.persist", Kind: "write"}, {Target: "jakarta.persistence.EntityManager.merge", Kind: "write"}, {Target: "jakarta.persistence.EntityManager.remove", Kind: "write"},
		{Target: "java.lang.Integer.parseInt", Kind: "input"}, {Target: "java.lang.Long.parseLong", Kind: "input"}, {Target: "java.lang.Double.parseDouble", Kind: "input"},
		{Target: "com.fasterxml.jackson.databind.ObjectMapper.readValue", Kind: "input"}, {Target: "com.fasterxml.jackson.databind.ObjectReader.readValue", Kind: "input"},
		{Target: "java.time.LocalDate.parse", Kind: "input"}, {Target: "java.time.LocalDateTime.parse", Kind: "input"}, {Target: "java.net.URI.create", Kind: "input"},
		{Target: "java.lang.ProcessBuilder.*", Kind: "command"}, {Target: "java.lang.Runtime.exec", Kind: "command"},
	},
	ReceiverMethods: map[string]string{
		"java.lang.Runtime.getRuntime":                          "java.lang.Runtime",
		"java.sql.DriverManager.getConnection":                  "java.sql.Connection",
		"java.sql.Connection.createStatement":                   "java.sql.Statement",
		"java.sql.Connection.prepareStatement":                  "java.sql.PreparedStatement",
		"java.sql.Connection.prepareCall":                       "java.sql.CallableStatement",
		"com.fasterxml.jackson.databind.ObjectMapper.reader":    "com.fasterxml.jackson.databind.ObjectReader",
		"com.fasterxml.jackson.databind.ObjectMapper.readerFor": "com.fasterxml.jackson.databind.ObjectReader",
		"java.lang.Class.forName":                               "java.lang.Class",
		"java.lang.Class.getMethod":                             "java.lang.reflect.Method",
		"java.lang.Class.getDeclaredMethod":                     "java.lang.reflect.Method",
	},
}

type AnnotationRule struct{ Kind, Property, Method string }

var Annotations = map[string]AnnotationRule{
	"picocli.CommandLine.Option":                             {Kind: "flag", Property: "names"},
	"picocli.CommandLine.Command":                            {Kind: "cmd", Property: "name"},
	"org.springframework.web.bind.annotation.RequestMapping": {Kind: "route"},
	"org.springframework.web.bind.annotation.GetMapping":     {Kind: "route", Method: "GET"},
	"org.springframework.web.bind.annotation.PostMapping":    {Kind: "route", Method: "POST"},
	"org.springframework.web.bind.annotation.PutMapping":     {Kind: "route", Method: "PUT"},
	"org.springframework.web.bind.annotation.PatchMapping":   {Kind: "route", Method: "PATCH"},
	"org.springframework.web.bind.annotation.DeleteMapping":  {Kind: "route", Method: "DELETE"},
	"javax.ws.rs.Path":   {Kind: "route", Property: "value"},
	"jakarta.ws.rs.Path": {Kind: "route", Property: "value"},
	"org.springframework.beans.factory.annotation.Value":                  {Kind: "cfg", Property: "value"},
	"org.springframework.boot.context.properties.ConfigurationProperties": {Kind: "cfg_fields"},
}

var Types = []string{
	"org.junit.Test", "org.junit.Assert", "org.junit.jupiter.api.Test", "org.junit.jupiter.api.Assertions",
	"org.junit.jupiter.params.ParameterizedTest", "org.junit.jupiter.params.provider.ValueSource", "org.junit.jupiter.params.provider.CsvSource", "org.junit.jupiter.params.provider.MethodSource", "org.junit.jupiter.params.provider.NullSource", "org.junit.jupiter.params.provider.EmptySource",
	"org.mockito.Mockito", "org.assertj.core.api.Assertions", "org.hamcrest.MatcherAssert",
	"java.lang.System", "java.lang.String", "java.lang.Object", "java.lang.Integer", "java.lang.Long", "java.lang.Double", "java.lang.Thread", "java.lang.Runtime", "java.lang.ProcessBuilder",
	"java.lang.Exception", "java.lang.RuntimeException", "java.lang.Throwable", "java.lang.Deprecated",
	"java.lang.Override", "java.lang.FunctionalInterface", "java.lang.Runnable", "java.lang.Boolean", "java.lang.Byte", "java.lang.Short", "java.lang.Character", "java.lang.Float", "java.lang.Number", "java.lang.Void",
	"java.util.concurrent.Callable", "java.util.function.Function", "java.util.function.Supplier", "java.util.function.Consumer", "java.util.function.Predicate", "java.util.function.UnaryOperator", "java.util.function.BinaryOperator", "java.util.function.BiFunction", "java.util.function.BiConsumer", "java.util.function.BiPredicate",
	"java.lang.Class", "java.lang.reflect.Method",
	"java.nio.file.Files", "java.nio.file.Paths", "java.nio.file.Path", "java.io.FileOutputStream", "java.io.FileWriter",
	"java.time.LocalDate", "java.time.LocalDateTime", "java.net.URI",
	"java.sql.Connection", "java.sql.DriverManager", "java.sql.Statement", "java.sql.PreparedStatement", "java.sql.CallableStatement",
	"java.util.concurrent.ExecutorService", "java.util.concurrent.CompletableFuture", "java.util.concurrent.Executors",
	"javax.persistence.EntityManager", "jakarta.persistence.EntityManager",
	"com.fasterxml.jackson.databind.ObjectMapper", "com.fasterxml.jackson.databind.ObjectReader",
	"org.springframework.web.bind.annotation.RequestMethod", "org.springframework.boot.autoconfigure.SpringBootApplication",
	"picocli.CommandLine",
}

func TestAssertion(target string) string {
	for _, name := range []string{"verify", "verifyNoInteractions", "verifyNoMoreInteractions"} {
		if target == "org.mockito.Mockito."+name {
			return "mock"
		}
	}
	for _, owner := range []string{"org.junit.Assert.", "org.junit.jupiter.api.Assertions."} {
		if strings.HasPrefix(target, owner+"assert") || target == owner+"fail" {
			return "result"
		}
	}
	if target == "org.assertj.core.api.Assertions.assertThat" || target == "org.hamcrest.MatcherAssert.assertThat" {
		return "result"
	}
	return ""
}

const Tags = `
(field_declaration declarator: (variable_declarator name: (identifier) @name)) @definition.variable
(constant_declaration declarator: (variable_declarator name: (identifier) @name)) @definition.constant
(record_declaration name: (identifier) @name) @definition.class
(annotation_type_declaration name: (identifier) @name) @definition.interface
(annotation_type_element_declaration name: (identifier) @name) @definition.method
(compact_constructor_declaration name: (identifier) @name) @definition.constructor
`
