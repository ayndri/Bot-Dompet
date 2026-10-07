package bot

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/ayndri/dompetku/ledger"
)

const budgetUsage = `Cara pakai:
<code>/budget</code> — lihat semua budget bulan ini
<code>/budget jajan 300rb</code> — atur budget bulanan
<code>/budget jajan hapus</code> — hapus budget`

// budget menangani /budget, /budget <kategori> <nominal>, dan
// /budget <kategori> hapus.
func (b *Bot) budget(ctx context.Context, chatID int64, args []string) (string, error) {
	if len(args) == 0 {
		return b.budgetOverview(ctx, chatID)
	}
	if len(args) != 2 {
		return budgetUsage, nil
	}

	cat, ok := ledger.FindCategory(ledger.Expense, args[0])
	if !ok {
		return fmt.Sprintf("Kategori <b>%s</b> nggak ada. Pilihannya: %s.",
			html.EscapeString(args[0]), strings.Join(ledger.Categories(ledger.Expense), ", ")), nil
	}

	if strings.EqualFold(args[1], "hapus") || args[1] == "0" {
		deleted, err := b.Store.DeleteBudget(ctx, chatID, cat)
		if err != nil {
			return "", err
		}
		if !deleted {
			return fmt.Sprintf("Kategori %s memang belum punya budget.", cat), nil
		}
		return fmt.Sprintf("🗑️ Budget %s dihapus.", cat), nil
	}

	amount, ok := ledger.ParseAmount(args[1])
	if !ok {
		return "Nominalnya nggak kebaca. " + budgetUsage, nil
	}
	if err := b.Store.SetBudget(ctx, chatID, ledger.Budget{Category: cat, Amount: amount}); err != nil {
		return "", err
	}
	spent, err := b.monthSpending(ctx, chatID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("✅ Budget <b>%s</b> diatur %s per bulan.\n\n%s",
		cat, ledger.Rupiah(amount), formatBudgetLine(ledger.Budget{Category: cat, Amount: amount}, spent[cat])), nil
}

func (b *Bot) budgetOverview(ctx context.Context, chatID int64) (string, error) {
	budgets, err := b.Store.Budgets(ctx, chatID)
	if err != nil {
		return "", err
	}
	if len(budgets) == 0 {
		return "Belum ada budget.\n\n" + budgetUsage, nil
	}
	spent, err := b.monthSpending(ctx, chatID)
	if err != nil {
		return "", err
	}

	from, _ := ThisMonth.Range(b.now())
	var s strings.Builder
	fmt.Fprintf(&s, "<b>🎯 Budget %s %d</b>\n", monthsLong[from.Month()-1], from.Year())
	for _, bg := range budgets {
		s.WriteString("\n" + formatBudgetLine(bg, spent[bg.Category]) + "\n")
	}
	return strings.TrimRight(s.String(), "\n"), nil
}

// monthSpending menjumlahkan pengeluaran bulan ini per kategori.
func (b *Bot) monthSpending(ctx context.Context, chatID int64) (map[string]int64, error) {
	from, to := ThisMonth.Range(b.now())
	totals, err := b.Store.Totals(ctx, chatID, from, to)
	if err != nil {
		return nil, err
	}
	spent := map[string]int64{}
	for _, t := range totals {
		if t.Kind == ledger.Expense {
			spent[t.Category] = t.Total
		}
	}
	return spent, nil
}

// budgetWarnings memeriksa apakah catatan yang baru disimpan membuat budget
// bulan ini melewati 80% atau 100%. Peringatan hanya muncul sekali per
// ambang, yaitu pada catatan yang melewatinya.
func (b *Bot) budgetWarnings(ctx context.Context, chatID int64, saved []ledger.Tx) ([]string, error) {
	from, to := ThisMonth.Range(b.now())
	added := map[string]int64{}
	for _, tx := range saved {
		if tx.Kind == ledger.Expense && !tx.CreatedAt.Before(from) && tx.CreatedAt.Before(to) {
			added[tx.Category] += tx.Amount
		}
	}
	if len(added) == 0 {
		return nil, nil
	}

	budgets, err := b.Store.Budgets(ctx, chatID)
	if err != nil || len(budgets) == 0 {
		return nil, err
	}
	spent, err := b.monthSpending(ctx, chatID)
	if err != nil {
		return nil, err
	}

	var warnings []string
	for _, bg := range budgets {
		add := added[bg.Category]
		if add == 0 {
			continue
		}
		after := spent[bg.Category]
		before := after - add
		switch {
		case before <= bg.Amount && after > bg.Amount:
			warnings = append(warnings, fmt.Sprintf("🚨 Budget <b>%s</b> bulan ini kelewatan %s (%s dari %s).",
				bg.Category, ledger.Rupiah(after-bg.Amount), ledger.Rupiah(after), ledger.Rupiah(bg.Amount)))
		case before*5 < bg.Amount*4 && after*5 >= bg.Amount*4:
			warnings = append(warnings, fmt.Sprintf("⚠️ Budget <b>%s</b> udah kepakai %d%% (%s dari %s). Sisa %s.",
				bg.Category, percent(after, bg.Amount), ledger.Rupiah(after), ledger.Rupiah(bg.Amount), ledger.Rupiah(bg.Amount-after)))
		}
	}
	return warnings, nil
}

func formatBudgetLine(bg ledger.Budget, spent int64) string {
	pct := percent(spent, bg.Amount)
	filled := min(int(pct/10), 10)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 10-filled)
	status := fmt.Sprintf("sisa %s", ledger.Rupiah(bg.Amount-spent))
	if spent > bg.Amount {
		status = fmt.Sprintf("🚨 lewat %s", ledger.Rupiah(spent-bg.Amount))
	}
	return fmt.Sprintf("<b>%s</b>: %s / %s (%d%%)\n<code>%s</code> %s",
		bg.Category, ledger.Rupiah(spent), ledger.Rupiah(bg.Amount), pct, bar, status)
}
