package lang

const langCount = 9

type entry struct {
	translations [langCount]string
}

var dictEntries []entry

func lookupWord(word string, l lang) (string, bool) {
	loadFromPage()
	if len(dictEntries) == 0 || word == "" {
		return "", false
	}
	low, high := 0, len(dictEntries)-1
	for low <= high {
		mid := low + (high-low)/2
		cmp := compareCaseInsensitive(word, dictEntries[mid].translations[EN])
		if cmp == 0 {
			res := dictEntries[mid].translations[int(l)]
			if res == "" {
				return "", false
			}
			return res, true
		}
		if cmp < 0 {
			high = mid - 1
		} else {
			low = mid + 1
		}
	}
	return "", false
}

func sortDict() {
	if len(dictEntries) < 2 {
		return
	}
	quicksort(dictEntries, 0, len(dictEntries)-1)
}

func quicksort(data []entry, low, high int) {
	if low < high {
		p := partition(data, low, high)
		quicksort(data, low, p)
		quicksort(data, p+1, high)
	}
}

func partition(data []entry, low, high int) int {
	pivot := data[(low+high)/2].translations[EN]
	i, j := low-1, high+1
	for {
		i++
		for compareCaseInsensitive(data[i].translations[EN], pivot) < 0 {
			i++
		}
		j--
		for compareCaseInsensitive(data[j].translations[EN], pivot) > 0 {
			j--
		}
		if i >= j {
			return j
		}
		data[i], data[j] = data[j], data[i]
	}
}

// compareCaseInsensitive compares two ASCII strings case-insensitively.
func compareCaseInsensitive(s1, s2 string) int {
	n1, n2 := len(s1), len(s2)
	n := n1
	if n2 < n {
		n = n2
	}
	for i := 0; i < n; i++ {
		c1, c2 := s1[i], s2[i]
		if c1 >= 'A' && c1 <= 'Z' {
			c1 += 32
		}
		if c2 >= 'A' && c2 <= 'Z' {
			c2 += 32
		}
		if c1 < c2 {
			return -1
		}
		if c1 > c2 {
			return 1
		}
	}
	if n1 < n2 {
		return -1
	}
	if n1 > n2 {
		return 1
	}
	return 0
}
