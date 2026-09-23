package render

import (
	"fmt"
	"io"
	"strings"

	"apm-investigator/internal/incident"
	"apm-investigator/internal/learning"
)

func Feedback(w io.Writer, result learning.Result) {
	f := result.Feedback
	fmt.Fprintf(w, "Feedback %s  %s\n", f.ID, f.Verdict)
	if f.Reflection == nil {
		fmt.Fprintln(w, "Reflection pending; retry with feedback reflect")
	} else {
		fmt.Fprintln(w, safe(f.Reflection.Summary))
		if result.Lesson == nil {
			fmt.Fprintln(w, "No reusable lesson proposed")
		}
	}
	if result.Lesson != nil {
		fmt.Fprintf(w, "Lesson %s  %s; inspect with memory show\n", result.Lesson.ID, result.Lesson.Status)
	}
	if f.Error != "" {
		fmt.Fprintln(w, "Error:", safe(f.Error))
	}
}

func Lesson(w io.Writer, l incident.Lesson) {
	fmt.Fprintf(w, "Lesson %s  %s (%s)\nWhen: %s\nCheck: %s\nWhy: %s\nSource run: %s\nFeedback: %s\nEvidence: %s\nScope: %s / %s / %s\n",
		l.ID, l.Status, l.Mode, safe(l.Draft.Condition), safe(l.Draft.Check), safe(l.Draft.Rationale), l.RunID, l.FeedbackID, safe(strings.Join(l.Draft.EvidenceIDs, ", ")), safe(l.Scope.MerchantID), safe(l.Scope.Method), safe(l.Scope.Environment))
}

func FeedbackDetails(w io.Writer, result learning.Result) {
	Feedback(w, result)
	fmt.Fprintf(w, "Source run: %s\nNote: %s\n", result.Feedback.RunID, safe(result.Feedback.Note))
}

func LessonList(w io.Writer, lessons []incident.Lesson) {
	for _, l := range lessons {
		fmt.Fprintf(w, "%s  %-8s %-4s %s\n", l.ID, l.Status, l.Mode, safe(l.Draft.Condition))
	}
}
