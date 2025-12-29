package roci

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/google/rpmpack"
)

// ParseRpmdepsOutput parses the out of the rpmdeps tool and returns RPMMetaData
// with deduplicated dependency relations (Requires, Provides, Recommends,
// etc.)
func ParseRpmdepsOutput(output string) (rpmpack.RPMMetaData, error) {
	deps := struct {
		requires   map[string]*rpmpack.Relation
		recommends map[string]*rpmpack.Relation
		provides   map[string]*rpmpack.Relation
		conflicts  map[string]*rpmpack.Relation
		obsoletes  map[string]*rpmpack.Relation
		suggests   map[string]*rpmpack.Relation
	}{
		requires:   make(map[string]*rpmpack.Relation),
		recommends: make(map[string]*rpmpack.Relation),
		provides:   make(map[string]*rpmpack.Relation),
		conflicts:  make(map[string]*rpmpack.Relation),
		obsoletes:  make(map[string]*rpmpack.Relation),
		suggests:   make(map[string]*rpmpack.Relation),
	}

	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if unicode.IsDigit(rune(line[0])) {
			continue
		}

		depType := line[0]
		depString := strings.TrimSpace(line[1:])

		var targetMap map[string]*rpmpack.Relation
		switch depType {
		case 'R':
			targetMap = deps.requires
		case 'r':
			targetMap = deps.recommends
		case 'P':
			targetMap = deps.provides
		case 'C':
			targetMap = deps.conflicts
		case 'O':
			targetMap = deps.obsoletes
		case 's':
			targetMap = deps.suggests
		case 'S', 'e', 'o':
			continue
		default:
			continue
		}

		if _, exists := targetMap[depString]; !exists {
			r, err := rpmpack.NewRelation(depString)
			if err != nil {
				return rpmpack.RPMMetaData{}, fmt.Errorf("invalid dependency %q: %w", depString, err)
			}
			targetMap[depString] = r
		}
	}

	return rpmpack.RPMMetaData{
		Requires:   mapValues(deps.requires),
		Recommends: mapValues(deps.recommends),
		Provides:   mapValues(deps.provides),
		Conflicts:  mapValues(deps.conflicts),
		Obsoletes:  mapValues(deps.obsoletes),
		Suggests:   mapValues(deps.suggests),
	}, nil
}

func mapValues(deps map[string]*rpmpack.Relation) []*rpmpack.Relation {
	if len(deps) == 0 {
		return nil
	}

	keys := make([]string, 0, len(deps))
	for k := range deps {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rels := make([]*rpmpack.Relation, 0, len(deps))
	for _, k := range keys {
		rels = append(rels, deps[k])
	}

	return rels
}
