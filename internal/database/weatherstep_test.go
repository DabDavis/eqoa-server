package database

import (
	"strings"
	"testing"
)

// A weather step's checks (StepProblems): its kind, strength, minutes, where
// and near, its text's length and its world; the controls pass.
func TestWeatherStepProblems(t *testing.T) {
	w := func(n int) *int { return &n }
	cases := []struct {
		s    TriggerStep
		want string // "" none
	}{
		{TriggerStep{Do: TriggerWeather, Weather: "snow"}, ""},
		{TriggerStep{Do: TriggerWeather, Weather: "sand", Strength: "light", Minutes: 30, Where: "near", Near: 3, Text: "Sand!"}, ""},
		{TriggerStep{Do: TriggerWeather, Weather: "whiteout", Where: "world", World: w(0), X: 1, Z: 2}, ""},
		{TriggerStep{Do: TriggerWeather, Weather: "clear", Minutes: 120}, ""},
		{TriggerStep{Do: TriggerWeather, Weather: "hail"}, `weather "hail"`},
		{TriggerStep{Do: TriggerWeather}, `weather ""`},
		{TriggerStep{Do: TriggerWeather, Weather: "rain", Strength: "brutal"}, "strength"},
		{TriggerStep{Do: TriggerWeather, Weather: "rain", Minutes: 1}, "minutes"},
		{TriggerStep{Do: TriggerWeather, Weather: "rain", Minutes: 121}, "minutes"},
		{TriggerStep{Do: TriggerWeather, Weather: "rain", Where: "near"}, "near 0"},
		{TriggerStep{Do: TriggerWeather, Weather: "rain", Where: "near", Near: 6}, "near 6"},
		{TriggerStep{Do: TriggerWeather, Weather: "rain", Where: "here", Near: 2}, "near is for where: near"},
		{TriggerStep{Do: TriggerWeather, Weather: "rain", Where: "zone"}, `where "zone"`},
		{TriggerStep{Do: TriggerWeather, Weather: "rain", Text: strings.Repeat("x", 201)}, "200 at most"},
		{TriggerStep{Do: TriggerWeather, Weather: "rain", World: w(30)}, "world 30"},
	}
	for _, c := range cases {
		ps := strings.Join(StepProblems([]TriggerStep{c.s}), "; ")
		if c.want == "" && ps != "" || c.want != "" && !strings.Contains(ps, c.want) {
			t.Errorf("%+v: %q, want %q", c.s, ps, c.want)
		}
	}
}
