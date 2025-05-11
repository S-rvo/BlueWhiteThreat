package deepdarkCTI

type TableEntry struct {
    Name        string `bson:"name" json:"name"`
    URL         string `bson:"url" json:"url"`
    Status      string `bson:"status" json:"status"`
    SourceFile  string `bson:"sourcefile" json:"sourcefile"`
    Description string `bson:"description" json:"description"`
}

type PRDiff struct {
    FileName        string
    AddedLines      []string
    // RemovedLines    []string // utile si on veut les supprimer
}

type PullRequest struct {
    Number   int
    Title    string
    Files    []string
    Diffs    []PRDiff
}

type Result struct {
    MarkdownData []TableEntry
    PullRequests []PullRequest
}

