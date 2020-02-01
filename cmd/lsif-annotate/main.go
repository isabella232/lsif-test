package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/alecthomas/kingpin"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/sourcegraph/lsif-test/elements"
)

const version = "0.1.0"

func main() {
	if err := realMain(); err != nil {
		fmt.Fprint(os.Stderr, fmt.Sprintf("error: %v\n", err))
		os.Exit(1)
	}
}

func realMain() error {
	app := kingpin.New("lsif-annotate", "lsif-annotate is an annotator for debugging LSIF indexer output.").Version(version)
	dumpFile := app.Arg("dump-file", "The LSIf output to validate.").Default("data.lsif").File()
	bufferCapacity := app.Flag("buffer-capacity", "Set the max line size.").Default("1000000").Int()
	docURIToAnnotate := app.Flag("docURI", "The document URI to annotate.").Required().String()

	_, err := app.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	defer (*dumpFile).Close()

	scanner := bufio.NewScanner(*dumpFile)
	scanner.Buffer(make([]byte, *bufferCapacity), *bufferCapacity)

	rangeIDsByDocID := make(map[elements.ID][]elements.ID)
	rangesByID := make(map[elements.ID]elements.DocumentRange)
	docsByURI := make(map[string]elements.Document)

	for scanner.Scan() {
		element, err := elements.ParseElement(scanner.Text())
		if err != nil {
			return errors.Wrap(err, "failed to parse element")
		}
		switch element.Label {
		case "range":
			rainge, err := elements.ParseDocumentRange(scanner.Text())
			if err != nil {
				return errors.Wrap(err, "failed to parse range")
			}
			rangesByID[rainge.ID] = *rainge
		case "document":
			doc, err := elements.ParseDocument(scanner.Text())
			if err != nil {
				return errors.Wrap(err, "failed to parse document")
			}
			docsByURI[doc.URI] = *doc
		case "contains":
			contains, err := elements.ParseEdge(scanner.Text())
			if err != nil {
				return errors.Wrap(err, "failed to parse document")
			}
			rangeIDsByDocID[contains.OutV] = contains.InVs
		}
	}

	docToAnnotate, ok := docsByURI[*docURIToAnnotate]
	if !ok {
		return fmt.Errorf("document %s not found", *docURIToAnnotate)
	}

	rangesByLine := make(map[int][]elements.DocumentRange)
	docToAnnotateRangeIDs := rangeIDsByDocID[docToAnnotate.ID]
	for _, rangeID := range docToAnnotateRangeIDs {
		if ranges, ok := rangesByLine[rangesByID[rangeID].Start.Line]; !ok {
			rangesByLine[rangesByID[rangeID].Start.Line] = []elements.DocumentRange{rangesByID[rangeID]}
		} else {
			ranges = append(ranges, rangesByID[rangeID])
		}
	}

	type Piece struct {
		text   string
		rainge *elements.DocumentRange
	}
	slice := func(line string, ranges []elements.DocumentRange) []Piece {
		return []Piece{Piece{text: line, rainge: nil}}
	}

	filepath := strings.TrimPrefix(*docURIToAnnotate, "file://")
	bytes, err := ioutil.ReadFile(filepath)
	if err != nil {
		return err
	}
	content := string(bytes)
	for linenumber, line := range strings.Split(content, "\n") {
		for _, piece := range slice(line, rangesByLine[linenumber]) {
			if piece.rainge == nil {
				fmt.Printf(piece.text)
			} else {
				fmt.Printf("**" + piece.text + "**")
			}
		}
		fmt.Printf("\n")
		for _, rainge := range rangesByLine[linenumber] {
			fmt.Printf(strings.Repeat(" ", rainge.Start.Character))
			color.Set(color.FgGreen)
			fmt.Printf(strings.Repeat("^", rainge.End.Character-rainge.Start.Character))
			fmt.Printf(" range %s", rainge.ID.Value)
			color.Unset()
			fmt.Println("")
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner: %v", err)
	}
	return nil
}
