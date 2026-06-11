package phaseisolation

import (
	"strings"
)

// AntiRationalization pairs a common agent excuse with a pre-written rebuttal.
type AntiRationalization struct {
	Excuse   string // The rationalization an agent might use
	Rebuttal string // Why it's wrong and what to do instead
}

// PhaseAntiRationalizations maps each phase to the rationalizations agents use
// to skip or shortcut that phase's required work.
var PhaseAntiRationalizations = map[PhaseID][]AntiRationalization{
	PhaseD1: {
		{
			Excuse:   "This problem is obvious, we don't need to validate it.",
			Rebuttal: "Obvious problems often dissolve under scrutiny or reveal the real problem is elsewhere. Validation takes an hour; rebuilding the wrong thing takes months.",
		},
		{
			Excuse:   "We already know the solution, why spend time defining the problem?",
			Rebuttal: "Solutions without validated problems waste effort on the wrong thing. Define the problem first; the solution will be sharper for it.",
		},
		{
			Excuse:   "We can validate after we build a prototype.",
			Rebuttal: "A prototype built on an unvalidated problem is a validated prototype of the wrong solution. Validate first, then prototype.",
		},
	},
	PhaseD2: {
		{
			Excuse:   "We know what to build; researching existing solutions wastes time.",
			Rebuttal: "Someone solved this already. Thirty minutes of research avoids three weeks of rebuilding something that already exists.",
		},
		{
			Excuse:   "Our use case is too specific for existing solutions to apply.",
			Rebuttal: "That's what everyone says before discovering the library that handles 80% of the problem. Verify the claim, then build what's truly missing.",
		},
		{
			Excuse:   "We've used this approach before; no need to re-evaluate alternatives.",
			Rebuttal: "Familiarity is not fitness. Evaluate whether the familiar approach still fits the current constraints before committing.",
		},
	},
	PhaseD3: {
		{
			Excuse:   "We'll figure out the approach as we go.",
			Rebuttal: "Approach decisions made under implementation pressure are the ones you regret at retro. Decide now while you can think clearly.",
		},
		{
			Excuse:   "There's only one real option anyway; a decision matrix is overkill.",
			Rebuttal: "If there's only one option, the decision matrix takes ten minutes and confirms it. That confirmation is worth having on record.",
		},
		{
			Excuse:   "The risks are obvious; we don't need to document them.",
			Rebuttal: "Risks that feel obvious now are the ones that get forgotten under delivery pressure. Write them down so mitigation stays intentional.",
		},
	},
	PhaseD4: {
		{
			Excuse:   "We'll figure out the API design during implementation.",
			Rebuttal: "API changes after implementation cost five times more and cascade across callers. Design the contract now; implementation becomes translation, not invention.",
		},
		{
			Excuse:   "Requirements will change, so detailed specs are wasteful.",
			Rebuttal: "Requirements always change. A spec captures what you know now so changes are deliberate, not accidental drift from a half-remembered conversation.",
		},
		{
			Excuse:   "The architecture is straightforward enough to skip documentation.",
			Rebuttal: "Architecture that seems obvious to you is a mystery to the next engineer. Document it while it's fresh or document it after a debugging session.",
		},
	},
	PhaseS4: {
		{
			Excuse:   "We discussed this informally; we don't need formal stakeholder alignment.",
			Rebuttal: "Informal alignment evaporates at the first handoff. Written confirmation survives vacations, team changes, and memory lapses.",
		},
		{
			Excuse:   "Stakeholders approved the rough idea; that's enough to proceed.",
			Rebuttal: "Rough approval covers rough ideas. If your design has evolved since the hallway conversation, get fresh explicit sign-off on what you're actually building.",
		},
	},
	PhaseS5: {
		{
			Excuse:   "We know enough to implement; a research phase is overkill.",
			Rebuttal: "Every 'we know enough' that skipped research has produced at least one rewrite. Spend an hour proving the assumption before betting a week on it.",
		},
		{
			Excuse:   "We'll discover unknowns during implementation.",
			Rebuttal: "Unknowns discovered during implementation become blockers that invalidate the plan and force context-switching at the worst moment. Find them now.",
		},
		{
			Excuse:   "Edge cases can be handled later; we'll prototype the happy path first.",
			Rebuttal: "Edge cases found during implementation collapse timelines. Edge cases found during research are design inputs.",
		},
	},
	PhaseS6: {
		{
			Excuse:   "The design is in my head; writing it down takes too long.",
			Rebuttal: "Design in your head evaporates overnight and is unavailable to teammates. Externalise it now while it's complete.",
		},
		{
			Excuse:   "I'll add diagrams once the code is working.",
			Rebuttal: "Diagrams of working code are documentation. Diagrams before code are design. Only the latter prevents bad architecture.",
		},
		{
			Excuse:   "The design is simple enough that diagrams would be redundant.",
			Rebuttal: "Simple designs that skip documentation become complex designs nobody can explain. If the diagram is trivial, drawing it takes five minutes.",
		},
	},
	PhaseS7: {
		{
			Excuse:   "We know what needs to be done; a formal plan wastes time.",
			Rebuttal: "Plans are not about knowing what to do. They're about discovering what you don't know before you start burning implementation time.",
		},
		{
			Excuse:   "We'll figure out task breakdown as we go.",
			Rebuttal: "Unplanned work consistently takes three times the estimate. Break it down now when the cost is negligible.",
		},
		{
			Excuse:   "The testing plan is obvious: just write tests.",
			Rebuttal: "Testing plans that write themselves produce exactly the gaps where bugs hide. Specify what to test and how before you forget the edge cases.",
		},
		{
			Excuse:   "We don't need a deployment plan for something this small.",
			Rebuttal: "Small changes with no deployment plan cause outages because the rollback procedure wasn't thought through. Plan it now, run it later.",
		},
	},
	PhaseS8: {
		{
			Excuse:   "I'll skip tests to save time.",
			Rebuttal: "Tests catch bugs that take ten times longer to fix once they reach production. Writing tests is not slower; shipping untested code is.",
		},
		{
			Excuse:   "I'll add documentation later.",
			Rebuttal: "Later never comes. Documentation written after memory fades is documentation nobody trusts. Write it while the implementation is fresh.",
		},
		{
			Excuse:   "Code review is just a formality for obvious changes.",
			Rebuttal: "Simple changes cause the worst outages because nobody checks them. The review is cheap. The outage is not.",
		},
		{
			Excuse:   "I'll fix the linter warnings after the main code works.",
			Rebuttal: "Linter warnings you defer are bugs you chose to delay. Fix them before they compound into technical debt nobody will touch.",
		},
		{
			Excuse:   "This is too simple to need tests.",
			Rebuttal: "Simple code without tests becomes complex code that can't be safely changed. The simpler it is, the faster the test is to write.",
		},
	},
	PhaseS9: {
		{
			Excuse:   "Unit tests passed; validation is redundant.",
			Rebuttal: "Unit tests verify units. Validation finds bugs in how units connect. Integration failures are exactly what unit tests miss.",
		},
		{
			Excuse:   "We tested the happy path; edge cases can be filed as issues.",
			Rebuttal: "Issues filed 'for later' stay open until they page you at 2am. Validate the edge cases now while the context is loaded.",
		},
		{
			Excuse:   "Performance validation can wait until we see a problem in production.",
			Rebuttal: "Performance problems found in production are outages. Performance problems found in validation are tuning tasks.",
		},
	},
	PhaseS10: {
		{
			Excuse:   "We'll add monitoring setup if something breaks.",
			Rebuttal: "You discover you need monitoring the moment something breaks in production, which is the worst time to set it up. Configure it before you deploy.",
		},
		{
			Excuse:   "The migration guide is only needed if something goes wrong.",
			Rebuttal: "Migration guides are written while you understand the change. After deployment, that understanding fades. Write it now.",
		},
		{
			Excuse:   "We can deploy without a rollback plan for something this small.",
			Rebuttal: "The need for rollback is inversely correlated with how much you plan for it. Small changes with no rollback plan are the ones that get stuck.",
		},
	},
	PhaseS11: {
		{
			Excuse:   "Everything went fine; a retro isn't needed this time.",
			Rebuttal: "When everything goes fine is exactly when you capture what to repeat. Wait for disaster to do your retros and you'll be too busy managing the disaster.",
		},
		{
			Excuse:   "We all know what happened; no need to write it down.",
			Rebuttal: "You know what happened. In six months you won't. And the next engineer on this codebase definitely doesn't. Write it down.",
		},
		{
			Excuse:   "The retro is just ceremony; we'll incorporate learnings informally.",
			Rebuttal: "Informal learnings don't update process. Only written retro findings become the next project's starting point.",
		},
	},
}

// GetAntiRationalizations returns the anti-rationalizations for a phase.
// Returns nil if no rationalizations are defined for that phase.
func GetAntiRationalizations(phaseID PhaseID) []AntiRationalization {
	return PhaseAntiRationalizations[phaseID]
}

// FormatAntiRationalizations formats anti-rationalizations as a markdown
// table for injection into phase system prompts.
func FormatAntiRationalizations(rationalizations []AntiRationalization) string {
	if len(rationalizations) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("| Excuse | Why it's wrong |\n")
	b.WriteString("|--------|----------------|\n")
	for _, ar := range rationalizations {
		b.WriteString("| ")
		b.WriteString(ar.Excuse)
		b.WriteString(" | ")
		b.WriteString(ar.Rebuttal)
		b.WriteString(" |\n")
	}
	return b.String()
}
