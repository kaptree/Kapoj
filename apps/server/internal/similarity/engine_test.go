package similarity

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

// at returns a fixed reference time offset by d, so tests can order candidates
// deterministically without real clock reads.
func at(d time.Duration) time.Time {
	return time.Unix(0, 0).Add(d)
}

// cppTemplate returns a C++ source with more than 100 normalized tokens. name
// is interpolated as an identifier, so renaming it does not change the
// normalized token stream (identifiers collapse to "ID").
func cppTemplate(name string) string {
	var b strings.Builder
	b.WriteString("#include <bits/stdc++.h>\n")
	b.WriteString("using namespace std;\n")
	fmt.Fprintf(&b, "int %s(int x) {\n", name)
	fmt.Fprintf(&b, "    int %s_acc = 0;\n", name)
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "    %s_acc += %d;\n", name, i)
	}
	fmt.Fprintf(&b, "    return %s_acc;\n", name)
	b.WriteString("}\n")
	b.WriteString("int main() {\n")
	fmt.Fprintf(&b, "    int %s_n = 20;\n", name)
	fmt.Fprintf(&b, "    int %s_r = %s(%s_n);\n", name, name, name)
	fmt.Fprintf(&b, "    cout << %s_r << endl;\n", name)
	b.WriteString("    return 0;\n")
	b.WriteString("}\n")
	return b.String()
}

// unrelatedCode returns a C++ source with more than 100 normalized tokens that
// shares no 20-token k-gram with cppTemplate, so the two compare as unrelated.
func unrelatedCode() string {
	var b strings.Builder
	b.WriteString("#include <vector>\n")
	b.WriteString("#include <string>\n")
	b.WriteString("using namespace std;\n")
	b.WriteString("struct Node {\n")
	b.WriteString("    string label;\n")
	b.WriteString("    Node* next;\n")
	b.WriteString("};\n")
	b.WriteString("int main() {\n")
	b.WriteString("    vector<int> values;\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "    values.push_back(%d);\n", i)
	}
	b.WriteString("    for (int i = 0; i < 10; i++) {\n")
	b.WriteString("        values[i] = values[i] * 2;\n")
	b.WriteString("    }\n")
	b.WriteString("    return 0;\n")
	b.WriteString("}\n")
	return b.String()
}

// boilerplate returns a large, structurally diverse C++ program used as a
// shared template or common prefix. Diversity matters: normalized tokens
// collapse identifiers to "ID" and literals to "NUM", so a repetitive body
// yields few distinct fingerprints. This program's many distinct constructs
// produce enough fingerprints to dominate any small body appended after it.
func boilerplate() string {
	return `#include <bits/stdc++.h>
using namespace std;
typedef long long ll;
typedef pair<int, int> pii;
const int INF = 1000000000;
int n, m;
vector<int> g[100010];
int dist[100010];
bool vis[100010];
ll fact[20];
int main() {
    ios::sync_with_stdio(false);
    cin.tie(nullptr);
    cin >> n >> m;
    for (int i = 0; i < m; i++) {
        int u, v;
        cin >> u >> v;
        g[u].push_back(v);
        g[v].push_back(u);
    }
    queue<int> q;
    q.push(1);
    vis[1] = true;
    while (!q.empty()) {
        int u = q.front();
        q.pop();
        for (int v : g[u]) {
            if (!vis[v]) {
                vis[v] = true;
                dist[v] = dist[u] + 1;
                q.push(v);
            }
        }
    }
    sort(dist + 1, dist + n + 1);
    ll ans = 0;
    for (int i = 1; i <= n; i++) {
        ans = (ans + dist[i]) % 1000000007;
    }
    cout << ans << endl;
    return 0;
}
`
}

// distinctBody returns a structurally distinct function body. Each kind starts
// with a different leading keyword and uses a different operator, so no two
// bodies share a 20-token k-gram with each other or with the boilerplate.
func distinctBody(kind int) string {
	lead := [...]string{"void", "double", "long", "char", "unsigned"}[kind]
	op := [...]string{"+", "-", "*", "/", "%"}[kind]
	var b strings.Builder
	fmt.Fprintf(&b, "%s solve() {\n", lead)
	fmt.Fprintf(&b, "    %s acc = 0;\n", lead)
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&b, "    acc = acc %s %d;\n", op, i)
	}
	b.WriteString("    return acc;\n")
	b.WriteString("}\n")
	return b.String()
}

func TestEngineDetectsRenamedCopy(t *testing.T) {
	original := Candidate{ID: 1, UserID: 10, Language: submission.LanguageCPP, CreatedAt: at(0), Code: cppTemplate("a")}
	copy := Candidate{ID: 2, UserID: 20, Language: submission.LanguageCPP, CreatedAt: at(time.Minute), Code: cppTemplate("b")}
	result, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
		Candidates: []Candidate{original, copy}, ThresholdPercent: 80,
	})
	require.NoError(t, err)
	require.Len(t, result.Pairs, 1)
	assert.EqualValues(t, 1, result.Pairs[0].OriginalSubmissionID)
	assert.EqualValues(t, 2, result.Pairs[0].SuspectSubmissionID)
	assert.EqualValues(t, 10, result.Pairs[0].OriginalUserID)
	assert.EqualValues(t, 20, result.Pairs[0].SuspectUserID)
	assert.GreaterOrEqual(t, result.Pairs[0].SimilarityPercent, 90.0)
}

func TestEngineIgnoresUnrelatedAndSameUser(t *testing.T) {
	a := Candidate{ID: 1, UserID: 10, Language: submission.LanguageCPP, CreatedAt: at(0), Code: cppTemplate("a")}
	b := Candidate{ID: 2, UserID: 10, Language: submission.LanguageCPP, CreatedAt: at(time.Minute), Code: cppTemplate("b")} // same user
	c := Candidate{ID: 3, UserID: 30, Language: submission.LanguageCPP, CreatedAt: at(2 * time.Minute), Code: unrelatedCode()}
	result, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
		Candidates: []Candidate{a, b, c}, ThresholdPercent: 80,
	})
	require.NoError(t, err)
	require.Empty(t, result.Pairs)
	assert.Equal(t, 3, result.SubmissionCount)
	assert.Equal(t, 0, result.SkippedCount)
}

func TestEngineRemovesBaseTemplate(t *testing.T) {
	base := boilerplate()
	a := Candidate{ID: 1, UserID: 10, Language: submission.LanguageCPP, CreatedAt: at(0), Code: base + distinctBody(0)}
	b := Candidate{ID: 2, UserID: 20, Language: submission.LanguageCPP, CreatedAt: at(time.Minute), Code: base + distinctBody(1)}

	// Without the base template the shared boilerplate dominates the body.
	without, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
		Candidates: []Candidate{a, b}, ThresholdPercent: 80,
	})
	require.NoError(t, err)
	require.Len(t, without.Pairs, 1)
	assert.GreaterOrEqual(t, without.Pairs[0].SimilarityPercent, 80.0)

	// With the base template removed, only the disjoint bodies remain.
	with, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
		Candidates: []Candidate{a, b}, ThresholdPercent: 80,
		BaseCode: base, BaseLanguage: submission.LanguageCPP,
	})
	require.NoError(t, err)
	assert.Empty(t, with.Pairs)
}

func TestEngineIgnoresHighFrequencyFingerprints(t *testing.T) {
	prefix := boilerplate()
	cands := make([]Candidate, 5)
	for i := range cands {
		cands[i] = Candidate{
			ID:        int64(i + 1),
			UserID:    int64(100 + i),
			Language:  submission.LanguageCPP,
			CreatedAt: at(time.Duration(i) * time.Minute),
			Code:      prefix + distinctBody(i),
		}
	}

	// Without the filter the shared prefix creates pairs between every body.
	without, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
		Candidates: cands, ThresholdPercent: 80,
	})
	require.NoError(t, err)
	require.NotEmpty(t, without.Pairs)

	// With the filter the common prefix (present in >50% of candidates) is
	// dropped, so the unrelated bodies no longer produce a pair.
	with, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
		Candidates: cands, ThresholdPercent: 80, IgnoreCommonCode: true,
	})
	require.NoError(t, err)
	assert.Empty(t, with.Pairs)
}

func TestEngineUsesEarlierSubmissionAsOriginal(t *testing.T) {
	later := Candidate{ID: 2, UserID: 20, Language: submission.LanguageCPP, CreatedAt: at(time.Minute), Code: cppTemplate("a")}
	earlier := Candidate{ID: 1, UserID: 10, Language: submission.LanguageCPP, CreatedAt: at(0), Code: cppTemplate("b")}
	result, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
		Candidates: []Candidate{later, earlier}, ThresholdPercent: 80,
	})
	require.NoError(t, err)
	require.Len(t, result.Pairs, 1)
	assert.EqualValues(t, 1, result.Pairs[0].OriginalSubmissionID)
	assert.EqualValues(t, 2, result.Pairs[0].SuspectSubmissionID)
	assert.EqualValues(t, 10, result.Pairs[0].OriginalUserID)
	assert.EqualValues(t, 20, result.Pairs[0].SuspectUserID)
}

func TestEngineErrorsWhenTooManyCandidates(t *testing.T) {
	cands := make([]Candidate, MaxSubmissionsPerProblem+1)
	for i := range cands {
		cands[i] = Candidate{ID: int64(i + 1), UserID: 1, Language: submission.LanguageCPP, CreatedAt: at(0), Code: cppTemplate("x")}
	}
	_, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
		Candidates: cands, ThresholdPercent: 80,
	})
	require.Error(t, err)
}

func TestEngineSkipsShortUnsupportedAndAnswer(t *testing.T) {
	valid := Candidate{ID: 1, UserID: 10, Language: submission.LanguageCPP, CreatedAt: at(0), Code: cppTemplate("a")}
	short := Candidate{ID: 2, UserID: 20, Language: submission.LanguageCPP, CreatedAt: at(0), Code: "int main() { return 0; }"}
	answer := Candidate{ID: 3, UserID: 30, Language: submission.LanguageAnswer, CreatedAt: at(0), Code: "A"}
	unsupported := Candidate{ID: 4, UserID: 40, Language: submission.Language("rust"), CreatedAt: at(0), Code: cppTemplate("b")}

	result, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
		Candidates: []Candidate{valid, short, answer, unsupported}, ThresholdPercent: 80,
	})
	require.NoError(t, err)
	assert.Empty(t, result.Pairs)
	assert.Equal(t, 1, result.SubmissionCount)
	assert.Equal(t, 3, result.SkippedCount)
}

func TestEngineOnlyComparesSameLanguage(t *testing.T) {
	cpp := Candidate{ID: 1, UserID: 10, Language: submission.LanguageCPP, CreatedAt: at(0), Code: cppTemplate("a")}
	py := Candidate{ID: 2, UserID: 20, Language: submission.LanguagePython3, CreatedAt: at(time.Minute), Code: cppTemplate("a")}
	result, err := NewWinnowingEngine().Compare(context.Background(), EngineRequest{
		Candidates: []Candidate{cpp, py}, ThresholdPercent: 80,
	})
	require.NoError(t, err)
	// Identical source text in two different languages must not pair.
	assert.Empty(t, result.Pairs)
}
