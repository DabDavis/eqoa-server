package skyisles

import "testing"

// Every known spot names its isle: each chain's essences (handlers/skypass.go's
// spawns, at their npcs rows' heights), the bosses and landmarks the forum's
// walkthroughs name for each isle, the main isle's Island of Wonder portal and
// the Isle of Wonder's landing.
func TestKnownSpots(t *testing.T) {
	for _, c := range []struct {
		what    string
		x, y, z float32
		want    string
	}{
		{"the Island of Wonder portal", 4824, 250, 5497, "Main Isle"},
		{"Liztin Misseliania", 4685, 250, 5398, "Main Isle"},
		{"essence of rain (rain key)", 4482.5, 244.7, 5293.2, "Main Isle"},
		{"essence of night (key of the ancients)", 4621.4, 270.8, 5729.6, "Main Isle"},
		{"essence of sun (sun key)", 5137.3, 261.3, 5188.3, "Main Isle"},
		{"the Astral Light, east edge", 5140, 250.1, 5455, "Main Isle"},
		{"the Isle of Wonder landing", 5000, 333.5, 4998, "Isle of Wonder"},
		{"Mark of the Majestic", 4980, 333, 4985, "Isle of Wonder"},
		{"Construct of Storms", 5105, 237, 5745, "Isle of the Constructs"},
		{"Astral Light, the constructs' isle", 5103.9, 236, 5778.9, "Isle of the Constructs"},

		{"essence of rain (Rain Key 1)", 4222.7, 233, 5714.6, "Rain Chain 1"},
		{"Artiokanis", 4234, 236, 5688, "Rain Chain 1"},
		{"essence of rain (Rain Key 2)", 4158.6, 180.9, 5122.5, "Rain Chain 2"},
		{"Trixie Longwings", 4169, 181, 5214, "Rain Chain 2"},
		{"Kaglanor", 4158, 180, 5159, "Rain Chain 2"},
		{"essence of rain (Rain Key 3)", 4638.6, 119.7, 4655.2, "Rain Chain 3"},
		{"Granavik", 4318, -69, 4700, "Rain Chain 3"},
		{"Rain 3 Portal", 4318, -69, 4700, "Rain Chain 3"},
		{"Singe Burnfeathers", 4611, 113, 4939, "Rain Chain 3"},
		{"essence of rain (Rain Key 4)", 4199.7, 80.9, 4148.3, "Rain Chain 4"},
		{"Spirit of Narciliisisin", 4246, 83, 4204, "Rain Chain 4"},
		{"essence of rain (Rain Key 5)", 4516.5, 30.9, 4271.6, "Rain Chain 5"},
		{"Chimareon the Tyrant", 4556, 31, 4276, "Rain Chain 5"},

		{"essence of day (Day Key 1)", 5597.3, 235.9, 5343.1, "Day Chain 1"},
		{"brightstar princess", 5640, 257, 5427, "Day Chain 1"},
		{"essence of day (Day Key 2)", 5380.8, 282.6, 4755.9, "Day Chain 2"},
		{"Imerzan", 5406, 282, 4751, "Day Chain 2"},
		{"Master Fiharaz", 5631, 403, 4663, "Day Chain 2"},
		{"essence of day (Day Key 3)", 5459.6, 381, 4237.5, "Day Chain 3"},
		{"torrential wind", 5538, 391, 4186, "Day Chain 3"},
		{"essence of day (Day Key 4)", 5054.4, 431.8, 4243.1, "Day Chain 4"},
		{"Randarin the Relentless", 5073, 431, 4244, "Day Chain 4"},
		{"Madam Watrya", 5640, 278, 5120, "Watrya's Isle"},

		{"essence of night (Night Key 1)", 4160.7, 613.3, 6404.1, "Night Chain 1"},
		{"Ancient Jester", 4175, 612, 6216, "Night Chain 1"},
		{"essence of night (Night Key 2)", 4441.2, 449.6, 6436.7, "Night Chain 2"},
		{"iron knight", 4442, 449, 6400, "Night Chain 2"},
		{"essence of night (Night Key 3)", 4250, 671.5, 6806.4, "Night Chain 3"},
		{"Night Maiden", 4250, 661, 6743, "Night Chain 3"},
		{"Arch Bishop Lancel", 4390, 662, 6608, "Night Chain 3"},
		{"essence of night (Night Key 4)", 4322.6, 354, 7754.4, "Night Chain 4"},
		{"dark baroness", 4155, 353, 7758, "Night Chain 4"},
		{"essence of night (Night Key 5)", 4400, 130.6, 7135.8, "Night Chain 5"},
		{"King Ruthra", 4499, 99, 7234, "Night Chain 5"},

		{"Ryini", 5668, 553, 6356, "Snow Chain 1"},
		{"Wind Mephit", 5674, 488, 6554, "Snow Chain 1"},
		{"essence of snow (Snow Key 2)", 5766.8, 238.3, 6702.1, "Snow Chain 2"},
		{"Tamis", 5550, 238, 6688, "Snow Chain 2"},
		{"Guenzi", 5701, 237, 6823, "Snow Chain 2"},
		{"Whirlwind", 5723, 237, 6928, "Snow Chain 2"},
		{"essence of snow (Snow Key 3)", 5906.3, 177.5, 7570.5, "Snow Chain 3"},
		{"glacial champion", 5884, 177, 7560, "Snow Chain 3"},
		{"essence of snow (Snow Key 4)", 5525.1, 96.1, 7708, "Snow Chain 4"},
		{"Enraged Aeyinar", 5509, 107, 7597, "Snow Chain 4"},

		{"essence of desolation (Desolation Key 1)", 4751.5, 568.9, 6418.5, "Desolation Chain 1"},
		{"Erych Arbiter", 4760, 569, 6249, "Desolation Chain 1"},
		{"essence of desolation (Desolation Key 2)", 5146.8, 616.6, 6620.1, "Desolation Chain 2"},
		{"warlord of Narcillian", 4657, 631, 6646, "Desolation Chain 2"},
		{"essence of desolation (Desolation Key 3)", 5116, 713.5, 7174.8, "Desolation Chain 3"},
		{"elite harpy queen", 5171, 713, 7162, "Desolation Chain 3"},
		{"essence of desolation (Desolation Key 4)", 4734.2, 884.5, 7688.3, "Desolation Chain 4"},
		{"Quintessence of Veeshan", 4728, 848, 7617, "Desolation Chain 4"},
	} {
		if got := Isle(c.x, c.y, c.z); got != c.want {
			t.Errorf("%s at (%g, %g, %g): %q, want %q", c.what, c.x, c.y, c.z, got, c.want)
		}
	}
}

// Place names the isle in the Plane of Sky alone; off every isle (the air far
// from any, under them all) the world's name stands alone.
func TestPlace(t *testing.T) {
	for _, c := range []struct {
		world   int
		x, y, z float32
		want    string
	}{
		{4, 4160.7, 613.3, 6404.1, "Plane of Sky (Night Chain 1)"},
		{4, 4824, 250, 5497, "Plane of Sky (Main Isle)"},
		{4, 5000, 333.5, 4998, "Plane of Sky (Isle of Wonder)"},
		{4, 0, 0, 0, "Plane of Sky"},
		{4, 5000, 0, 9000, "Plane of Sky"},
		{0, 4160.7, 613.3, 6404.1, "Tunaria"},
	} {
		name := "Plane of Sky"
		if c.world == 0 {
			name = "Tunaria"
		}
		if got := Place(c.world, name, c.x, c.y, c.z); got != c.want {
			t.Errorf("world %d (%g, %g, %g): %q, want %q", c.world, c.x, c.y, c.z, got, c.want)
		}
	}
}

// Snow 1's mountain stands over snow 2's igloo village: the same (x, z) is
// snow 1 on the mountain and snow 2 on the village's ground below.
func TestOverlapByHeight(t *testing.T) {
	if got := Isle(5766.8, 238.3, 6702.1); got != "Snow Chain 2" {
		t.Errorf("snow 2's essence: %q", got)
	}
	if got := Isle(5766.8, 520, 6702.1); got != "Snow Chain 1" {
		t.Errorf("above it, on the mountain: %q", got)
	}
}
