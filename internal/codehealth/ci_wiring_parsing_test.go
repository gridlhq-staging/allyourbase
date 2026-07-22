package codehealth

// Shared parsing helpers for the CI wiring contract tests: Makefile recipes,
// executable shell command extraction, and GitHub Actions workflow structure.
// These are the single owner of that parsing; contract tests assert against
// them rather than re-implementing their own matchers.

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type githubActionsWorkflow struct {
	Defaults    githubActionsDefaults       `yaml:"defaults"`
	On          map[string]yaml.Node        `yaml:"on"`
	Permissions map[string]string           `yaml:"permissions"`
	Jobs        map[string]githubActionsJob `yaml:"jobs"`
}

type githubActionsPushTrigger struct {
	Branches []string `yaml:"branches"`
	Paths    []string `yaml:"paths"`
}

type githubActionsJob struct {
	ContinueOnError yaml.Node             `yaml:"continue-on-error"`
	Defaults        githubActionsDefaults `yaml:"defaults"`
	If              string                `yaml:"if"`
	Needs           yaml.Node             `yaml:"needs"`
	Permissions     map[string]string     `yaml:"permissions"`
	Steps           []githubActionsStep   `yaml:"steps"`
	TimeoutMinutes  int                   `yaml:"timeout-minutes"`
}

type githubActionsStep struct {
	Name             string            `yaml:"name"`
	Uses             string            `yaml:"uses"`
	Run              string            `yaml:"run"`
	With             map[string]string `yaml:"with"`
	ContinueOnError  yaml.Node         `yaml:"continue-on-error"`
	If               string            `yaml:"if"`
	WorkingDirectory string            `yaml:"working-directory"`
}

type githubActionsDefaults struct {
	Run githubActionsRunDefaults `yaml:"run"`
}

type githubActionsRunDefaults struct {
	WorkingDirectory string `yaml:"working-directory"`
}

const (
	githubActionsCheckoutAction       = "actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd"
	githubActionsSetupNode22Action    = "actions/setup-node@48b55a011bda9f5d6aeb4c2d9c7362e8dae4041e"
	githubActionsSetupGoAction        = "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16"
	githubActionsUploadArtifactAction = "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a"
)

func readRepoText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}
	return string(data)
}

func makeTargetRecipe(t *testing.T, repoRoot, target string) string {
	t.Helper()
	makefile := readRepoText(t, filepath.Join(repoRoot, "Makefile"))
	return extractMakeTargetRecipe(makefile, target)
}

func extractMakeTargetRecipe(makefile, target string) string {
	var recipe []string
	inTarget := false
	targetPattern := regexp.MustCompile(`^` + regexp.QuoteMeta(target) + `(?:\s+[^:]+)?\s*:`)
	anyTargetPattern := regexp.MustCompile(`^[A-Za-z0-9_.-]+(?:\s+[A-Za-z0-9_.-]+)*\s*:`)
	for _, line := range strings.Split(makefile, "\n") {
		if targetPattern.MatchString(line) {
			inTarget = true
			continue
		}
		if inTarget && anyTargetPattern.MatchString(line) {
			break
		}
		if inTarget && strings.HasPrefix(line, "\t") {
			recipe = append(recipe, strings.TrimPrefix(line, "\t"))
		}
	}
	return strings.Join(recipe, "\n")
}

func makefileDeclaresPhony(makefile, target string) bool {
	for _, line := range strings.Split(makefile, "\n") {
		declaration, found := strings.CutPrefix(line, ".PHONY:")
		if found && containsStringField(strings.Fields(declaration), target) {
			return true
		}
	}
	return false
}

func makeTargetPrerequisites(makefile, target string) []string {
	return parseMakefilePrerequisiteGraph(makefile)[target]
}

func parseMakefilePrerequisiteGraph(makefile string) map[string][]string {
	graph := make(map[string][]string)
	for _, line := range strings.Split(makefile, "\n") {
		targetsText, prerequisitesText, found := strings.Cut(line, ":")
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "#") ||
			!found || strings.Contains(targetsText, "=") {
			continue
		}
		prerequisites := strings.Fields(strings.SplitN(prerequisitesText, "#", 2)[0])
		for _, target := range strings.Fields(targetsText) {
			graph[target] = append([]string(nil), prerequisites...)
		}
	}
	return graph
}

func makeTargetReachesPrerequisite(makefile, target, prerequisite string) (bool, string) {
	graph := parseMakefilePrerequisiteGraph(makefile)
	visiting := make(map[string]bool)
	var walk func(string, []string) (bool, string)
	walk = func(current string, path []string) (bool, string) {
		if current == prerequisite {
			return true, ""
		}
		dependencies, ok := graph[current]
		if !ok {
			return false, "missing Make target " + current
		}
		if visiting[current] {
			return false, "cyclic Make dependency path: " + strings.Join(append(path, current), " -> ")
		}
		visiting[current] = true
		defer delete(visiting, current)
		for _, dependency := range dependencies {
			if _, ok := graph[dependency]; dependency != prerequisite && !ok {
				return false, "indeterminate Make dependency path: unresolved prerequisite " + dependency
			}
			reached, problem := walk(dependency, append(path, current))
			if reached || problem != "" {
				return reached, problem
			}
		}
		return false, ""
	}
	return walk(target, nil)
}

func containsStringField(fields []string, want string) bool {
	for _, field := range fields {
		if field == want {
			return true
		}
	}
	return false
}

func recipeCommandBlocks(recipe string) []string {
	var blocks []string
	var current strings.Builder
	fallbackBraceDepth := 0
	for _, line := range strings.Split(recipe, "\n") {
		trimmedRight := strings.TrimRight(line, " \t")
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(trimmedRight)
		fallbackBraceDepth = shellFallbackBraceDepthAfter(line, fallbackBraceDepth)
		if !strings.HasSuffix(trimmedRight, `\`) && fallbackBraceDepth == 0 {
			blocks = append(blocks, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}
	return blocks
}

func shellFunctionBody(t *testing.T, script, name string) string {
	t.Helper()
	startPattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\(\) \{$`)
	start := startPattern.FindStringIndex(script)
	if start == nil {
		t.Fatalf("shell function %s not found", name)
	}
	body := script[start[1]:]
	endPattern := regexp.MustCompile(`(?m)^\}$`)
	end := endPattern.FindStringIndex(body)
	if end == nil {
		t.Fatalf("shell function %s has no closing brace", name)
	}
	return body[:end[0]]
}

func executableShellCommandBlocks(script string) []string {
	var commands []string
	for _, block := range recipeCommandBlocks(script) {
		command := normalizeExecutableShellBlock(block)
		if command == "" {
			continue
		}
		commands = append(commands, command)
	}
	return commands
}

func normalizeExecutableShellBlock(block string) string {
	block = strings.ReplaceAll(block, "\\\n", " ")
	var lines []string
	for _, line := range strings.Split(block, "\n") {
		trimmed, executable := normalizeExecutableShellLine(line)
		if !executable {
			return ""
		}
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
	}
	return strings.Join(lines, "\n")
}

func normalizeExecutableShellLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(stripShellComment(line))
	if trimmed == "" {
		return "", true
	}
	prefixEnd := 0
	ignoreErrors := false
	for prefixEnd < len(trimmed) {
		switch trimmed[prefixEnd] {
		case '@', '+':
			prefixEnd++
		case '-':
			ignoreErrors = true
			prefixEnd++
		default:
			return strings.TrimSpace(trimmed[prefixEnd:]), !ignoreErrors
		}
	}
	return "", !ignoreErrors
}

func stripShellComment(line string) string {
	return strings.SplitN(line, "#", 2)[0]
}

func shellCommandBlockIndex(commands []string, args []string) int {
	for index, command := range commands {
		if shellBlockRunsCommandWithArgs(command, args) {
			return index
		}
	}
	return -1
}

func shellBlockRunsCommandWithArgs(block string, args []string) bool {
	for _, segment := range shellCommandSegments(block) {
		for _, candidate := range []string{segment, stripShellEnvAssignments(segment)} {
			if shellSegmentStartsWithFields(candidate, args) {
				return true
			}
			if len(args) > 0 && shellSegmentStartsWithFields(candidate, append([]string{"bash"}, args...)) {
				return true
			}
		}
	}
	return false
}

// stripShellEnvAssignments drops the leading NAME=value prefix of a command so
// `OLLAMA_HOST=127.0.0.1:11434 ollama serve` still matches `ollama serve`.
func stripShellEnvAssignments(segment string) string {
	fields := strings.Fields(segment)
	for len(fields) > 0 && shellFieldIsEnvAssignment(fields[0]) {
		fields = fields[1:]
	}
	return strings.Join(fields, " ")
}

func shellFieldIsEnvAssignment(field string) bool {
	name, _, found := strings.Cut(field, "=")
	if !found || name == "" {
		return false
	}
	for _, char := range name {
		if char != '_' && !('A' <= char && char <= 'Z') && !('a' <= char && char <= 'z') && !('0' <= char && char <= '9') {
			return false
		}
	}
	return true
}

func shellCommandSegments(block string) []string {
	var segments []string
	for _, commandList := range splitShellTopLevelCommandLists(block) {
		parts, operators := splitShellCommandList(commandList)
		for index, part := range parts {
			if index > 0 {
				// Everything past a `||` only runs when the left side failed, and
				// everything past a known-dead `&&` never runs at all.
				if operators[index-1] == "||" || shellSegmentAlwaysFails(normalizeShellSegment(parts[index-1])) {
					break
				}
			}
			if index < len(operators) && operators[index] == "||" &&
				shellSegmentNeutralizesFailure(normalizeShellSegment(parts[index+1])) {
				// `cmd || true` runs cmd but discards its exit status, so cmd
				// cannot gate anything.
				continue
			}
			appendShellSegment(&segments, part)
		}
	}
	return segments
}

func splitShellTopLevelCommandLists(block string) []string {
	var commandLists []string
	start := 0
	braceDepth := 0
	for index := 0; index < len(block); index++ {
		char := block[index]
		nextBraceDepth := shellFallbackBraceDepthAt(block, index, braceDepth)
		if nextBraceDepth != braceDepth {
			braceDepth = nextBraceDepth
			continue
		}
		if braceDepth == 0 && isShellCommandListDelimiter(rune(char)) {
			commandLists = append(commandLists, block[start:index])
			start = index + 1
		}
	}
	return append(commandLists, block[start:])
}

func shellFallbackBraceDepthAfter(block string, depth int) int {
	for index := 0; index < len(block); index++ {
		depth = shellFallbackBraceDepthAt(block, index, depth)
	}
	return depth
}

func shellFallbackBraceDepthAt(block string, index, depth int) int {
	if depth == 0 && isShellFallbackBraceStart(block, index) {
		return 1
	}
	if depth == 0 || !isShellBraceToken(block, index) {
		return depth
	}
	if block[index] == '{' {
		return depth + 1
	}
	return depth - 1
}

func isShellFallbackBraceStart(block string, index int) bool {
	if block[index] != '{' || !isShellBraceToken(block, index) {
		return false
	}
	prefix := strings.TrimRight(block[:index], " \t\r\n")
	return strings.HasSuffix(prefix, "||")
}

func isShellBraceToken(block string, index int) bool {
	char := block[index]
	if char != '{' && char != '}' {
		return false
	}
	beforeBoundary := index == 0 || strings.ContainsRune(" \t\r\n;|&", rune(block[index-1]))
	afterBoundary := index+1 == len(block) || strings.ContainsRune(" \t\r\n;|&", rune(block[index+1]))
	return beforeBoundary && afterBoundary
}

// splitShellCommandList splits one command list on `&&` and `||`, returning the
// commands alongside the operators that separate them.
func splitShellCommandList(commandList string) (parts []string, operators []string) {
	start := 0
	braceDepth := 0
	for index := 0; index < len(commandList)-1; index++ {
		nextBraceDepth := shellFallbackBraceDepthAt(commandList, index, braceDepth)
		if nextBraceDepth != braceDepth {
			braceDepth = nextBraceDepth
			continue
		}
		if braceDepth > 0 {
			continue
		}
		operator := commandList[index : index+2]
		if operator != "&&" && operator != "||" {
			continue
		}
		parts = append(parts, commandList[start:index])
		operators = append(operators, operator)
		start = index + len(operator)
		index++
	}
	return append(parts, commandList[start:]), operators
}

// shellSegmentNeutralizesFailure reports whether using segment as a `||`
// fallback swallows the preceding command's failure instead of recording it.
func shellSegmentNeutralizesFailure(segment string) bool {
	fields := strings.Fields(segment)
	if len(fields) == 0 {
		return false
	}
	if len(fields) == 1 && shellFieldCapturesStatus(fields[0]) {
		return false
	}
	if shellSegmentPropagatesFailure(segment) {
		return false
	}
	switch fields[0] {
	case "true", ":", "echo", "printf":
		return true
	case "exit", "return":
		if len(fields) < 2 {
			return true
		}
		status, err := strconv.Atoi(strings.Trim(fields[1], `"'`))
		return err == nil && status == 0
	default:
		return true
	}
}

func shellSegmentPropagatesFailure(segment string) bool {
	groupBody := strings.TrimSpace(segment)
	if strings.HasPrefix(groupBody, "{") && strings.HasSuffix(groupBody, "}") {
		groupBody = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(groupBody, "{"), "}"))
	}
	for _, commandList := range strings.FieldsFunc(groupBody, isShellCommandListDelimiter) {
		parts, _ := splitShellCommandList(commandList)
		if len(parts) == 0 {
			continue
		}
		command := normalizeShellSegment(parts[0])
		if shellSegmentStartsWithCommand(command, "die") || shellSegmentTerminatesWithFailure(command) {
			return true
		}
	}
	return false
}

func shellSegmentTerminatesWithFailure(segment string) bool {
	fields := strings.Fields(segment)
	if len(fields) < 2 || (fields[0] != "exit" && fields[0] != "return") {
		return false
	}
	status, err := strconv.Atoi(strings.Trim(fields[1], `"'`))
	return err == nil && status != 0
}

func shellFieldCapturesStatus(field string) bool {
	name, value, found := strings.Cut(field, "=")
	if !found || name == "" {
		return false
	}
	for _, char := range name {
		if char != '_' && !('A' <= char && char <= 'Z') && !('a' <= char && char <= 'z') && !('0' <= char && char <= '9') {
			return false
		}
	}
	return value == "$?" || value == "$$?"
}

func isShellCommandListDelimiter(r rune) bool {
	return r == '\n' || r == ';'
}

func appendShellSegment(segments *[]string, segment string) {
	normalized := normalizeShellSegment(segment)
	if normalized == "" {
		return
	}
	if shellSegmentStartsWithCommand(normalized, "echo") || shellSegmentStartsWithCommand(normalized, "printf") {
		return
	}
	*segments = append(*segments, normalized)
}

func normalizeShellSegment(segment string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(segment), "()"))
}

func shellSegmentStartsWithCommand(segment, command string) bool {
	fields := strings.Fields(segment)
	return len(fields) > 0 && fields[0] == command
}

func shellSegmentStartsWithFields(segment string, want []string) bool {
	fields := strings.Fields(segment)
	if len(fields) < len(want) {
		return false
	}
	for index, field := range want {
		if strings.Trim(fields[index], `"'`) != field {
			return false
		}
	}
	return true
}

func shellSegmentAlwaysFails(segment string) bool {
	fields := strings.Fields(segment)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "false":
		return true
	case "exit", "return":
		if len(fields) < 2 {
			return false
		}
		status, err := strconv.Atoi(strings.Trim(fields[1], `"'`))
		return err == nil && status != 0
	default:
		return false
	}
}

func jobNeeds(job githubActionsJob, neededJob string) bool {
	switch job.Needs.Kind {
	case yaml.ScalarNode:
		return job.Needs.Value == neededJob
	case yaml.SequenceNode:
		for _, node := range job.Needs.Content {
			if node.Value == neededJob {
				return true
			}
		}
	}
	return false
}

func jobHasRunStep(job githubActionsJob, command string) bool {
	return workflowJobHasRunStep(githubActionsWorkflow{
		Jobs: map[string]githubActionsJob{"job": job},
	}, "job", command)
}

const makeTargetJavaScriptStamp = "ui/dist/.stamp"

func TestCIWorkflowPinsEveryActionToACommitSHA(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	var workflow githubActionsWorkflow
	if err := yaml.Unmarshal([]byte(readRepoText(t, workflowPath)), &workflow); err != nil {
		t.Fatalf("decode %s failed: %v", workflowPath, err)
	}

	for jobName, job := range workflow.Jobs {
		for index, step := range job.Steps {
			if strings.TrimSpace(step.Uses) == "" {
				continue
			}
			if !githubActionsRefIsCommitPinned(step.Uses) {
				t.Fatalf("jobs.%s.steps[%d].uses = %q, want immutable owner/name@40-hex commit SHA", jobName, index, step.Uses)
			}
		}
	}
}

func requireWorkflowJobPreparesJavaScriptPrerequisites(t *testing.T, workflow githubActionsWorkflow, makefile, jobName, target string) {
	t.Helper()
	if problem := workflowJobJavaScriptPrerequisiteProblem(workflow, makefile, jobName, target); problem != "" {
		t.Error(problem)
	}
}

func TestWorkflowJobJavaScriptPrerequisiteProblem(t *testing.T) {
	nodeStep := githubActionsStep{Uses: githubActionsSetupNode22Action, With: map[string]string{"node-version": "22"}}
	pnpmStep := githubActionsStep{Name: "Enable pnpm", Run: "corepack enable pnpm"}
	makeStep := githubActionsStep{Run: "make test-target"}
	wrongNode := githubActionsStep{Uses: githubActionsSetupNode22Action, With: map[string]string{"node-version": "20"}}
	conditionalNode := nodeStep
	conditionalNode.If = "success()"
	nonGatingPnpm := pnpmStep
	nonGatingPnpm.ContinueOnError = yaml.Node{Kind: yaml.ScalarNode, Value: "true"}
	transitiveMakefile := "test-target: build\nbuild: " + makeTargetJavaScriptStamp + "\n" + makeTargetJavaScriptStamp + ":\n"
	cases := []struct {
		name, makefile string
		steps          []githubActionsStep
		want           []string
	}{
		{"direct dependency with ordered setup", "test-target: " + makeTargetJavaScriptStamp + "\n", []githubActionsStep{nodeStep, pnpmStep, makeStep}, nil},
		{"transitive dependency with ordered setup", transitiveMakefile, []githubActionsStep{nodeStep, pnpmStep, makeStep}, nil},
		{"missing node", transitiveMakefile, []githubActionsStep{pnpmStep, makeStep}, []string{"missing unconditional root-level Node 22 setup-node"}},
		{"wrong node version", transitiveMakefile, []githubActionsStep{wrongNode, pnpmStep, makeStep}, []string{"missing unconditional root-level Node 22 setup-node"}},
		{"missing pnpm", transitiveMakefile, []githubActionsStep{nodeStep, makeStep}, []string{"missing unconditional root-level corepack enable pnpm"}},
		{"setup after make", transitiveMakefile, []githubActionsStep{makeStep, nodeStep, pnpmStep}, []string{"missing unconditional root-level Node 22 setup-node", "missing unconditional root-level corepack enable pnpm"}},
		{"pnpm before node", transitiveMakefile, []githubActionsStep{pnpmStep, nodeStep, makeStep}, []string{"Node 22 setup-node must run before corepack enable pnpm"}},
		{"conditional node setup", transitiveMakefile, []githubActionsStep{conditionalNode, pnpmStep, makeStep}, []string{"missing unconditional root-level Node 22 setup-node"}},
		{"non gating pnpm setup", transitiveMakefile, []githubActionsStep{nodeStep, nonGatingPnpm, makeStep}, []string{"missing unconditional root-level corepack enable pnpm"}},
		{"dependency cycle", "test-target: build\nbuild: test-target\n", []githubActionsStep{nodeStep, pnpmStep, makeStep}, []string{"cyclic Make dependency path"}},
		{"unresolved dependency", "test-target: missing-target\n", []githubActionsStep{nodeStep, pnpmStep, makeStep}, []string{"unresolved prerequisite missing-target"}},
	}
	for _, tc := range cases {
		workflow := githubActionsWorkflow{Jobs: map[string]githubActionsJob{"job": {Steps: tc.steps}}}
		problem := workflowJobJavaScriptPrerequisiteProblem(workflow, tc.makefile, "job", "test-target")
		if len(tc.want) == 0 && problem != "" {
			t.Fatalf("%s: problem = %q, want none", tc.name, problem)
		}
		for _, want := range tc.want {
			if !strings.Contains(problem, want) {
				t.Fatalf("%s: problem = %q, want substring %q", tc.name, problem, want)
			}
		}
	}
}

func workflowJobJavaScriptPrerequisiteProblem(workflow githubActionsWorkflow, makefile, jobName, target string) string {
	reachesStamp, dependencyProblem := makeTargetReachesPrerequisite(makefile, target, makeTargetJavaScriptStamp)
	if dependencyProblem != "" {
		return jobName + " cannot derive Make prerequisites for " + target + ": " + dependencyProblem
	}
	if !reachesStamp {
		return ""
	}
	if _, ok := workflow.Jobs[jobName]; !ok {
		return "workflow is missing jobs." + jobName
	}
	makeStepIndex := workflowJobRunStepIndex(workflow, jobName, "make "+target)
	nodeStepIndex := workflowJobStepIndex(workflow, jobName, githubActionsStepSetsUpNode22)
	pnpmStepIndex := workflowJobStepIndex(workflow, jobName, githubActionsStepEnablesPnpm)
	var problems []string
	if makeStepIndex < 0 {
		problems = append(problems, "missing gating make "+target+" step")
	}
	if nodeStepIndex < 0 || (makeStepIndex >= 0 && nodeStepIndex > makeStepIndex) {
		problems = append(problems, "missing unconditional root-level Node 22 setup-node before make "+target)
	}
	if pnpmStepIndex < 0 || (makeStepIndex >= 0 && pnpmStepIndex > makeStepIndex) {
		problems = append(problems, "missing unconditional root-level corepack enable pnpm before make "+target)
	}
	if nodeStepIndex >= 0 && pnpmStepIndex >= 0 && nodeStepIndex > pnpmStepIndex {
		problems = append(problems, "Node 22 setup-node must run before corepack enable pnpm")
	}
	if len(problems) == 0 {
		return ""
	}
	return jobName + " must prepare JavaScript prerequisites because " +
		target + " reaches " + makeTargetJavaScriptStamp + ": " + strings.Join(problems, "; ")
}

func workflowJobHasRunStep(workflow githubActionsWorkflow, jobName, command string) bool {
	return workflowJobRunStepIndex(workflow, jobName, command) >= 0
}

func workflowJobRunStepIndex(workflow githubActionsWorkflow, jobName, command string) int {
	job, ok := workflow.Jobs[jobName]
	if !ok {
		return -1
	}
	for index, step := range job.Steps {
		if !githubActionsStepIsRootGating(workflow.Defaults, job, step) {
			continue
		}
		for _, block := range executableShellCommandBlocks(step.Run) {
			if shellBlockRunsCommandWithArgs(block, strings.Fields(command)) {
				return index
			}
		}
	}
	return -1
}

func workflowJobStepIndex(workflow githubActionsWorkflow, jobName string, matches func(githubActionsStep) bool) int {
	job, ok := workflow.Jobs[jobName]
	if !ok {
		return -1
	}
	for index, step := range job.Steps {
		if githubActionsStepIsRootGating(workflow.Defaults, job, step) && matches(step) {
			return index
		}
	}
	return -1
}

func githubActionsStepSetsUpNode22(step githubActionsStep) bool {
	return strings.TrimSpace(step.Uses) == githubActionsSetupNode22Action &&
		strings.TrimSpace(step.With["node-version"]) == "22"
}

func githubActionsStepEnablesPnpm(step githubActionsStep) bool {
	for _, block := range executableShellCommandBlocks(step.Run) {
		if shellBlockRunsCommandWithArgs(block, []string{"corepack", "enable", "pnpm"}) {
			return true
		}
	}
	return false
}

func githubActionsStepIsRootGating(workflowDefaults githubActionsDefaults, job githubActionsJob, step githubActionsStep) bool {
	if strings.TrimSpace(githubActionsEffectiveWorkingDirectory(workflowDefaults, job.Defaults, step.WorkingDirectory)) != "" {
		return false
	}
	if githubActionsContinueOnErrorEnabled(job.ContinueOnError) {
		return false
	}
	if githubActionsContinueOnErrorEnabled(step.ContinueOnError) {
		return false
	}
	// A job- or step-level `if` makes the command skippable, so it can no longer
	// gate the workflow on every run of the triggering events.
	if strings.TrimSpace(job.If) != "" || strings.TrimSpace(step.If) != "" {
		return false
	}
	return true
}

func githubActionsEffectiveWorkingDirectory(workflowDefaults, jobDefaults githubActionsDefaults, stepWorkingDirectory string) string {
	if strings.TrimSpace(stepWorkingDirectory) != "" {
		return stepWorkingDirectory
	}
	if strings.TrimSpace(jobDefaults.Run.WorkingDirectory) != "" {
		return jobDefaults.Run.WorkingDirectory
	}
	return workflowDefaults.Run.WorkingDirectory
}

func githubActionsContinueOnErrorEnabled(node yaml.Node) bool {
	if node.Kind == 0 {
		return false
	}
	return strings.TrimSpace(strings.ToLower(node.Value)) != "false"
}

func githubActionsPermissionGrantsWrite(permissions map[string]string, name string) bool {
	return strings.TrimSpace(strings.ToLower(permissions[name])) == "write"
}

func requireJobUsesAction(t *testing.T, job githubActionsJob, actionRef string) {
	t.Helper()
	if _, ok := findJobActionStep(job, actionRef); ok {
		return
	}
	t.Fatalf("expected job to use %s", actionRef)
}

func findJobActionStep(job githubActionsJob, actionRef string) (githubActionsStep, bool) {
	for _, step := range job.Steps {
		if strings.TrimSpace(step.Uses) == actionRef {
			return step, true
		}
	}
	return githubActionsStep{}, false
}

func githubActionsJobActionRefs(job githubActionsJob) []string {
	var refs []string
	for _, step := range job.Steps {
		if ref := strings.TrimSpace(step.Uses); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func githubActionsRefIsCommitPinned(ref string) bool {
	parts := strings.Split(strings.TrimSpace(ref), "@")
	if len(parts) != 2 {
		return false
	}
	return regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(parts[1])
}
