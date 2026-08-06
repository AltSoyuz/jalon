# Heading one

## Heading two

### Heading three

A paragraph on one line.
A second line of the same paragraph.

- a bullet
- a bullet with `inline code`
- a bullet with a [relative link](../docs/adr-3.md)
- a bullet with an [external link](https://example.com/a?b=1&c=2)

```go
func main() {
	if a < b && c > d {
		fmt.Println("## not a heading")
	}
}
```

Escaping: <script>alert(1)</script> & "quotes" and 5 < 6.

An [unsafe link](javascript:alert(1)) stays literal text.

| a table | is |
| ------- | -- |
| outside | the subset |

1. an ordered list is outside the subset too

  - a nested bullet is outside the subset

<b>Inline HTML</b> is escaped, never interpreted.
