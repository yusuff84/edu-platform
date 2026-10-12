// Package service — доменная логика: биллинг и проверка заданий.
package service

// Split делит сумму платежа на комиссию платформы и долю учителя.
// commissionPct ограничен 0..90 по контракту настроек.
func Split(amountCents, commissionPct int) (platformFee, teacherAmount int) {
	if commissionPct < 0 {
		commissionPct = 0
	}
	if commissionPct > 90 {
		commissionPct = 90
	}
	if amountCents < 0 {
		amountCents = 0
	}
	platformFee = amountCents * commissionPct / 100
	return platformFee, amountCents - platformFee
}
