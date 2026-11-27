// Package contracttest fournit aux services les vérifications de divergence
// vis-à-vis du contrat : tags bson des structs et littéraux de clés Redis
// présents dans le code de production (scripts Lua compris).
package contracttest

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// BSONTags renvoie le nom bson de chaque champ exporté de la struct v.
func BSONTags(v any) []string {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	var tags []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("bson"), ",")
		if name == "" || name == "-" {
			continue
		}
		tags = append(tags, name)
	}
	return tags
}

// RequireBSONTagsIn échoue si un tag bson de v n'appartient pas à allowed.
func RequireBSONTagsIn(t testing.TB, v any, allowed []string) {
	t.Helper()
	set := toSet(allowed)
	for _, tag := range BSONTags(v) {
		if !set[tag] {
			t.Errorf("%T: champ bson %q absent du contrat pkg/contract", v, tag)
		}
	}
}

// RequireBSONTagsEqual échoue si les tags bson de v diffèrent exactement de want.
func RequireBSONTagsEqual(t testing.TB, v any, want []string) {
	t.Helper()
	got := sorted(BSONTags(v))
	exp := sorted(want)
	if strings.Join(got, ",") != strings.Join(exp, ",") {
		t.Errorf("%T: tags bson %v, contrat %v", v, got, exp)
	}
}

// keyLiteral capture un nom de clé entre guillemets simples, doubles ou
// backquotes, y compris dans les scripts Lua embarqués.
var keyLiteral = regexp.MustCompile("['\"`]([A-Za-z0-9_:]+)['\"`]")

// KeyLiterals renvoie, par fichier, les littéraux du code de production (hors
// _test.go) situé sous root qui satisfont match.
func KeyLiterals(t testing.TB, root string, match func(string) bool) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range keyLiteral.FindAllStringSubmatch(string(data), -1) {
			if match(m[1]) {
				out[path] = append(out[path], m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parcours de %s: %v", root, err)
	}
	return out
}

// RequireForeignKeysShared échoue si le code sous root emploie une clé d'un
// espace de noms appartenant à un autre service (foreign) sans qu'elle soit
// déclarée dans shared. Un préfixe partagé terminé par ":" couvre ses suffixes.
func RequireForeignKeysShared(t testing.TB, root string, foreign func(string) bool, shared []string) {
	t.Helper()
	for path, keys := range KeyLiterals(t, root, foreign) {
		for _, key := range keys {
			if !isShared(key, shared) {
				t.Errorf("%s: clé Redis %q d'un autre service absente de pkg/contract", path, key)
			}
		}
	}
}

// RequireKeysReferenced échoue si l'une des clés n'apparaît pas en littéral
// dans le code de production sous root. Côté propriétaire d'une clé, cela
// détecte un renommage du contrat que les scripts Lua n'auraient pas suivi.
func RequireKeysReferenced(t testing.TB, root string, keys []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, found := range KeyLiterals(t, root, func(string) bool { return true }) {
		for _, key := range found {
			seen[key] = true
		}
	}
	for _, key := range keys {
		if !seen[key] {
			t.Errorf("clé Redis %q du contrat introuvable sous %s", key, root)
		}
	}
}

func isShared(key string, shared []string) bool {
	for _, s := range shared {
		if key == s || (strings.HasSuffix(s, ":") && (key == strings.TrimSuffix(s, ":") || strings.HasPrefix(key, s))) {
			return true
		}
	}
	return false
}

func toSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func sorted(xs []string) []string {
	c := append([]string(nil), xs...)
	sort.Strings(c)
	return c
}
