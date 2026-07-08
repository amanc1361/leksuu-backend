package usecase

// Lesson templates encode a recommended step sequence per CEFR level, distilled
// from how Busuu, Mondly and ELSA structure lessons (Presentation → Practice →
// Production). They carry STRUCTURE only (kind + skill) — never user-facing
// prose — so the message catalog stays the single source of truth for labels.

type TemplateStep struct {
	Kind     string `json:"kind"`     // item kind or exercise type
	Skill    string `json:"skill"`    // listening | reading | writing | speaking | vocab | presentation
	Exercise bool   `json:"exercise"` // true => an interactive exercise type; false => a content item
}

type LessonTemplate struct {
	Level string         `json:"level"`
	Steps []TemplateStep `json:"steps"`
}

func tItem(kind, skill string) TemplateStep { return TemplateStep{Kind: kind, Skill: skill} }
func tEx(kind, skill string) TemplateStep {
	return TemplateStep{Kind: kind, Skill: skill, Exercise: true}
}

func lessonTemplates() []LessonTemplate {
	a1 := []TemplateStep{
		tItem("image", "presentation"),
		tItem("text", "vocab"),
		tEx("listen_choose", "listening"),
		tEx("match_image", "vocab"),
		tItem("mini_story", "listening"),
		tEx("true_false", "reading"),
		tEx("sentence_builder", "writing"),
	}
	a2 := []TemplateStep{
		tItem("image", "presentation"),
		tItem("text", "vocab"),
		tItem("dialogue", "listening"),
		tEx("listen_choose", "listening"),
		tEx("fill_blank", "writing"),
		tEx("match", "vocab"),
		tEx("true_false", "reading"),
		tEx("dictation", "listening"),
		tEx("sentence_builder", "writing"),
	}
	b1 := []TemplateStep{
		tItem("text", "reading"),
		tEx("true_false", "reading"),
		tItem("podcast", "listening"),
		tEx("listen_choose", "listening"),
		tEx("dictation", "listening"),
		tEx("fill_blank", "writing"),
		tEx("reorder", "writing"),
		tEx("speak", "speaking"),
	}
	bc := []TemplateStep{
		tItem("text", "reading"),
		tEx("true_false", "reading"),
		tItem("podcast", "listening"),
		tEx("listen_choose", "listening"),
		tEx("fill_blank", "writing"),
		tEx("reorder", "writing"),
		tEx("speak", "speaking"),
	}
	return []LessonTemplate{
		{Level: "A1", Steps: a1},
		{Level: "A2", Steps: a2},
		{Level: "B1", Steps: b1},
		{Level: "B2", Steps: bc},
		{Level: "C1", Steps: bc},
		{Level: "C2", Steps: bc},
	}
}
