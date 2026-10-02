package cmd

const aiGuide = `# md — share markdown through md.erk.im

Uploads markdown files or folders and prints a link. Only the link goes to
stdout; status lines go to stderr. Links are readable by anyone who has them,
so never share secrets.

## Commands
- md share <file|dir|->... [--ttl 24h|7d|30d|never] [--title "..."] [--slides] [--name notes.md]
    Upload .md/.mdx/.markdown files or folders. "-" reads one document from stdin.
    Always pass --ttl when scripting, otherwise an interactive prompt may appear.
    --slides prints a link that opens as a slide deck (split on --- or headings).
- md share <paths>... --update [--ttl ...]   Re-upload to the share made earlier for the same paths: same link, new content, expiry kept unless --ttl.
- md share <paths>... --id <link|id>          Replace a specific share.
- md list [--json]                            Shares owned by the token's account (can lag ~1 min after creation).
- md extend <link|id> [7d] [--from-now]       Push back the expiry (max 30 days from now).
- md keep <link|id>                           Never expire (personal token only).
- md rename <link|id> ["title"]               Set or reset the title.
- md unshare <link|id>...                     Delete.
- md setup [--token mdr_...]                  Save the token. MD_TOKEN and MD_URL override the saved values.

## Limits
500 files, 5 MB of markdown per share; images are not uploaded.
`
