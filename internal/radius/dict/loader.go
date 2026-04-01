package dict

import "layeh.com/radius/dictionary"

func LoadDictionary(root string) (*dictionary.Dictionary, error) {
	parser := &dictionary.Parser{
		Opener: &dictionary.FileSystemOpener{
			Root: root,
		},
	}

	return parser.ParseFile("dictionary")
}
