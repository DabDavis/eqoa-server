package entity

// The shadow log's summary (the GM panel's formula_shadow_summary): how the
// formula's figures compare with the built-in ones, by path, attacker kind
// and attacker level band. It reads the file (and the one rotated before it)
// off the tick, and shows figures alone: no formula's text, and none of the
// records' shadow_ tags but the one a summary is asked to group by (Tag).
// The figures compared are a line's comparable ones, as each would land
// (builtin_after_effects, formula_after_effects); a line written before
// those were recorded, its builtin and value.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// ShadowSummaryOptions narrow a summary.
type ShadowSummaryOptions struct {
	Path  string    // a path's name (npc_melee ...); "" for all
	Since time.Time // records at or after it; zero for all
	Top   int       // how many of the largest differences; 0: 10
	// Tag groups the hits again by the value of one of the formula's
	// shadow_ steps (its name without the prefix): ByTag's "path / tag=value",
	// a hit without it under "tag=(none)". "" for none.
	Tag string
}

// ShadowStats are one group's figures. Covered hits are those the formula
// modelled: it ran, and it drew or gave a figure other than the built-in
// (a formula that returns base for a hit it does not cover draws nothing).
type ShadowStats struct {
	Key       string  `json:"key"` // "path / attacker kind / L<band>"
	Hits      int     `json:"hits"`
	Covered   int     `json:"covered"`
	Failed    int     `json:"failed"`
	Fallbacks int     `json:"fallbacks"` // lines whose built-in figure stood for the formula's (a failure, or a shot the file leaves on the built-in)
	BuiltMean float64 `json:"builtin_mean"`
	BuiltMed  float64 `json:"builtin_median"`
	BuiltP90  float64 `json:"builtin_p90"`
	FormMean  float64 `json:"formula_mean"`
	FormMed   float64 `json:"formula_median"`
	FormP90   float64 `json:"formula_p90"`
	Ratio     float64 `json:"ratio"` // formula_mean / builtin_mean over the covered hits (0 when none)

	built, form []float64
}

// ShadowDiff is one of the largest differences.
type ShadowDiff struct {
	At       string `json:"t"`
	Path     string `json:"path"`
	Attacker string `json:"attacker"` // name (kind, level)
	Defender string `json:"defender"`
	Builtin  int    `json:"builtin"`
	Formula  int    `json:"formula"`
	Diff     int    `json:"diff"`
}

// ShadowSummary is a summary of the shadow log.
type ShadowSummary struct {
	Files    []string       `json:"files"`
	Lines    int            `json:"lines"`
	Bad      int            `json:"unreadable_lines"`
	First    string         `json:"first,omitempty"`
	Last     string         `json:"last,omitempty"`
	Total    ShadowStats    `json:"total"`
	ByPath   []*ShadowStats `json:"by_path"`
	Groups   []*ShadowStats `json:"groups"`
	ByTag    []*ShadowStats `json:"by_tag,omitempty"`
	Largest  []ShadowDiff   `json:"largest_differences"`
	Failures map[string]int `json:"failures,omitempty"` // a run's failure (part and kind) -> hits
	// FallbackReasons is "path / fallback" -> lines (ranged_builtin, negative, ...)
	FallbackReasons map[string]int `json:"fallback_reasons,omitempty"`
}

// shadowBand is a level's band: 1-10, 11-20, ... 61+.
func shadowBand(level int) string {
	switch {
	case level <= 0:
		return "L?"
	case level > 60:
		return "L61+"
	}
	lo := (level-1)/10*10 + 1
	return fmt.Sprintf("L%d-%d", lo, lo+9)
}

func inNum(in map[string]any, k string) int {
	if f, ok := in[k].(float64); ok {
		return int(f)
	}
	return 0
}

func inStr(in map[string]any, k string) string {
	s, _ := in[k].(string)
	return s
}

// SummarizeShadowLog reads the shadow log at path (and path.1 before it).
func SummarizeShadowLog(path string, o ShadowSummaryOptions) (*ShadowSummary, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("no combat.formula_shadow_log is set")
	}
	if o.Top <= 0 {
		o.Top = 10
	}
	s := &ShadowSummary{Total: ShadowStats{Key: "all"}}
	paths, groups, tags := map[string]*ShadowStats{}, map[string]*ShadowStats{}, map[string]*ShadowStats{}
	var diffs []ShadowDiff
	for _, f := range []string{path + ".1", path} {
		fh, err := os.Open(f)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		s.Files = append(s.Files, f)
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var r ShadowRecord
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				s.Bad++
				continue
			}
			if o.Path != "" && r.Path != o.Path {
				continue
			}
			if !o.Since.IsZero() {
				if t, err := time.Parse(time.RFC3339Nano, r.At); err == nil && t.Before(o.Since) {
					continue
				}
			}
			s.Lines++
			if s.First == "" {
				s.First = r.At
			}
			s.Last = r.At
			kind := inStr(r.In, "a_kind")
			gk := r.Path + " / " + kind + " / " + shadowBand(inNum(r.In, "a_level"))
			if paths[r.Path] == nil {
				paths[r.Path] = &ShadowStats{Key: r.Path}
			}
			if groups[gk] == nil {
				groups[gk] = &ShadowStats{Key: gk}
			}
			built, form := r.Builtin, r.Value
			if r.FormulaAt != "" {
				built, form = r.BuiltinAfter, r.FormulaAfter
			}
			covered := r.Fail == "" && (r.Draws > 0 || form != built)
			sts := []*ShadowStats{&s.Total, paths[r.Path], groups[gk]}
			if o.Tag != "" {
				tk := r.Path + " / " + o.Tag + "=(none)"
				if v, ok := r.Tags[o.Tag]; ok {
					tk = fmt.Sprintf("%s / %s=%v", r.Path, o.Tag, v)
				}
				if tags[tk] == nil {
					tags[tk] = &ShadowStats{Key: tk}
				}
				sts = append(sts, tags[tk])
			}
			for _, st := range sts {
				st.Hits++
				if r.Fail != "" {
					st.Failed++
				}
				if r.Fallback != "" {
					st.Fallbacks++
				}
				if covered {
					st.Covered++
					st.built = append(st.built, float64(built))
					st.form = append(st.form, float64(form))
				}
			}
			if r.Fallback != "" {
				if s.FallbackReasons == nil {
					s.FallbackReasons = map[string]int{}
				}
				s.FallbackReasons[r.Path+" / "+r.Fallback]++
			}
			if r.Fail != "" {
				if s.Failures == nil {
					s.Failures = map[string]int{}
				}
				s.Failures[r.Fail]++
			}
			if covered && form != built {
				diffs = append(diffs, ShadowDiff{At: r.At, Path: r.Path,
					Attacker: fmt.Sprintf("%s (%s, %d)", inStr(r.In, "a_name"), kind, inNum(r.In, "a_level")),
					Defender: fmt.Sprintf("%s (%s, %d)", inStr(r.In, "d_name"), inStr(r.In, "d_kind"), inNum(r.In, "d_level")),
					Builtin:  built, Formula: form, Diff: form - built})
				if len(diffs) > 4*o.Top+64 { // keep the largest only, as it reads
					sortDiffs(diffs)
					diffs = diffs[:o.Top]
				}
			}
		}
		err = sc.Err()
		fh.Close()
		if err != nil {
			return nil, err
		}
	}
	if len(s.Files) == 0 {
		return nil, fmt.Errorf("no shadow log yet (%s)", path)
	}
	s.Total.finish()
	for _, st := range paths {
		st.finish()
		s.ByPath = append(s.ByPath, st)
	}
	for _, st := range groups {
		st.finish()
		s.Groups = append(s.Groups, st)
	}
	for _, st := range tags {
		st.finish()
		s.ByTag = append(s.ByTag, st)
	}
	sort.Slice(s.ByTag, func(i, j int) bool { return s.ByTag[i].Key < s.ByTag[j].Key })
	sort.Slice(s.ByPath, func(i, j int) bool { return s.ByPath[i].Key < s.ByPath[j].Key })
	sort.Slice(s.Groups, func(i, j int) bool { return s.Groups[i].Key < s.Groups[j].Key })
	sortDiffs(diffs)
	if len(diffs) > o.Top {
		diffs = diffs[:o.Top]
	}
	s.Largest = diffs
	return s, nil
}

func sortDiffs(d []ShadowDiff) {
	sort.SliceStable(d, func(i, j int) bool { return shadowAbs(d[i].Diff) > shadowAbs(d[j].Diff) })
}

func shadowAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (st *ShadowStats) finish() {
	st.BuiltMean, st.BuiltMed, st.BuiltP90 = shadowFigures(st.built)
	st.FormMean, st.FormMed, st.FormP90 = shadowFigures(st.form)
	if st.BuiltMean > 0 {
		st.Ratio = math.Round(st.FormMean/st.BuiltMean*1000) / 1000
	}
	st.built, st.form = nil, nil
}

// shadowFigures is a sample's mean, median and 90th percentile (nearest rank).
func shadowFigures(x []float64) (mean, median, p90 float64) {
	if len(x) == 0 {
		return 0, 0, 0
	}
	sort.Float64s(x)
	sum := 0.0
	for _, v := range x {
		sum += v
	}
	rank := func(p float64) float64 {
		i := int(math.Ceil(p*float64(len(x)))) - 1
		return x[max(0, min(len(x)-1, i))]
	}
	return math.Round(sum/float64(len(x))*100) / 100, rank(0.5), rank(0.9)
}
