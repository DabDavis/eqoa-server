package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/eqoa/server/internal/entity"
)

func writeSurvey(path string, rows []entity.DisperseRow) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	b := bufio.NewWriter(f)
	fmt.Fprintln(b, "id\tworld\tname\tlevel\tnpctype\tmodel\tsize\tx\ty\tz\tfacing\twanders\twhy\twhycode\tword\tonground\tinwater\tkeeps\tbody\tguard\tintown\ttownsman\tpathed\tcamp\tspawnonly\tpetof")
	bi := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	for _, r := range rows {
		fmt.Fprintf(b, "%d\t%d\t%s\t%d\t%d\t%d\t%.3f\t%.3f\t%.3f\t%.3f\t%.4f\t%d\t%s\t%d\t%s\t%d\t%d\t%s\t%.3f\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
			r.ID, r.World, r.Name, r.Level, r.NPCType, r.Model, r.Size, r.X, r.Y, r.Z, r.Facing, bi(r.Wanders), r.Why, r.WhyCode, r.Word,
			bi(r.OnGround), bi(r.InWater), r.Keeps, r.Body, bi(r.Guard), bi(r.InTown), bi(r.Townsman), bi(r.Pathed), bi(r.Camp), bi(r.SpawnOnly), r.PetOf)
	}
	b.Flush()
	f.Close()
}
