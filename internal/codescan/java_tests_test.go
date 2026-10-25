package codescan

import (
	"slices"
	"testing"
)

func TestJavaTestIdentitiesAndAssertions(t *testing.T) {
	file := scanSources(t, map[string]string{"src/test/java/RangeTest.java": `import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.*;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;
import static org.assertj.core.api.Assertions.assertThat;
class RangeTest {
 @Test void ordinary(){ assertEquals(1, parse(1)); }
 @ParameterizedTest @ValueSource(ints={1,2}) void values(int value){ assertEquals(value, parse(value)); }
 @ParameterizedTest @CsvSource({"1,1", "2,2"}) void csv(int input, int output){ assertThat(parse(input)).isEqualTo(output); }
 @Test void mock(){ verify(save).run(); verifyNoMoreInteractions(save); }
 @ParameterizedTest @MethodSource("rows") void dynamic(int value){ assertTrue(value>0); }
}
`}).Files["src/test/java/RangeTest.java"]
	want := []string{"RangeTest#ordinary", "RangeTest#values[1]", "RangeTest#values[2]", "RangeTest#csv[1]", "RangeTest#csv[2]", "RangeTest#mock", "RangeTest#dynamic"}
	var names []string
	for _, test := range file.Tests {
		names = append(names, test.Name)
		if len(test.Assertions) == 0 {
			t.Fatal("missing assertions", test, file.Missing)
		}
		for _, assertion := range test.Assertions {
			kind := "result"
			if test.Name == "RangeTest#mock" {
				kind = "mock"
			}
			if assertion.Kind != kind {
				t.Fatal(test, assertion)
			}
		}
	}
	if !slices.Equal(names, want) || len(file.Tests[len(file.Tests)-1].Unknown) == 0 {
		t.Fatal(names, file.Missing)
	}
}

func TestJavaLocalTestAnnotationAndAssertionShadow(t *testing.T) {
	file := scanSources(t, map[string]string{"src/test/java/TestCases.java": `import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.assertTrue;
class Cases {
 @Test void callsLocal(){ assertTrue(true); }
 void assertTrue(boolean value){}
}
class Other {
 @interface Test{}
 @Test void notJUnit(){}
}`}).Files["src/test/java/TestCases.java"]
	if len(file.Tests) != 1 || len(file.Tests[0].Assertions) != 0 {
		t.Fatal(file.Tests, file.Missing)
	}
}
