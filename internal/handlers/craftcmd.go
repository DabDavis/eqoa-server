package handlers

import (
	"strconv"

	"github.com/eqoa/server/internal/session"
)

// The GM's tradeskill commands (craft.go), the GM's own character's alone.

// craftSkillCommand is #craftskill: with nothing after it, the GM's six
// skills; <skill 0-5> <level 1-10> [rank 0-9] sets one (its XP into the rank
// 0), kept, and the client's slots with it (0x1102 14: the Recipe Book's
// title shows them).
func (h *ChatHandler) craftSkillCommand(sess *session.Session, args []string) {
	const usage = "Usage: #craftskill <skill 0-5> <level 1-10> [rank 0-9]"
	if h.craft == nil {
		h.gmReply(sess, "#craftskill: tradeskills are not served here")
		return
	}
	if len(args) == 0 {
		lines, err := h.craft.Skills(sess)
		if err != nil {
			h.gmReply(sess, "#craftskill: "+err.Error())
			return
		}
		for _, l := range lines {
			h.gmReply(sess, l)
		}
		h.gmReply(sess, usage+" ("+craftSkillList()+")")
		return
	}
	nums := make([]int, 0, 3)
	for _, a := range args {
		n, err := strconv.Atoi(a)
		if err != nil {
			h.gmReply(sess, usage)
			return
		}
		nums = append(nums, n)
	}
	if len(nums) < 2 || len(nums) > 3 {
		h.gmReply(sess, usage)
		return
	}
	rank := 0
	if len(nums) == 3 {
		rank = nums[2]
	}
	said, err := h.craft.SetSkill(sess, nums[0], nums[1], rank)
	if err != nil {
		h.gmReply(sess, "#craftskill: "+err.Error())
		return
	}
	h.gmReply(sess, said)
}

// craftLearnCommand is #craftlearn <recipe id | all [skill]>: recipes put at
// the end of the GM's own book.
func (h *ChatHandler) craftLearnCommand(sess *session.Session, args []string) {
	if h.craft == nil {
		h.gmReply(sess, "#craftlearn: tradeskills are not served here")
		return
	}
	said, err := h.craft.Learn(sess, args)
	if err != nil {
		h.gmReply(sess, "#craftlearn: "+err.Error())
		return
	}
	h.gmReply(sess, said)
}
