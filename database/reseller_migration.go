package database

import (
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

// The marker prevents removed assignments/accounts from being recreated on restart.
func migrateLegacyResellers() error {
	return db.Transaction(func(tx *gorm.DB) error {
		const name = "IndependentResellerAccountsV3"
		var count int64
		if err := tx.Model(&model.HistoryOfSeeders{}).Where("seeder_name = ?", name).Count(&count).Error; err != nil || count > 0 {
			return err
		}
		var legacy []model.ResellerAccess
		if err := tx.Order("id").Find(&legacy).Error; err != nil {
			return err
		}
		for _, old := range legacy {
			var inbound model.Inbound
			err := tx.First(&inbound, old.InboundID).Error
			if err != nil && !IsNotFound(err) {
				return err
			}
			owner := inbound.UserId
			if owner == 0 {
				var user model.User
				if err := tx.Order("id").First(&user).Error; err != nil {
					return err
				}
				owner = user.Id
			}
			account := model.ResellerAccount{ID: old.ID, AdminID: owner, Enabled: old.Enabled,
				Path: old.Path, Username: old.Username, PasswordHash: old.PasswordHash, Version: old.Version}
			if err := tx.Create(&account).Error; err != nil {
				return err
			}
			if inbound.Id != 0 {
				if err := tx.Create(&model.ResellerInbound{ResellerID: account.ID, InboundID: inbound.Id}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.ResellerAudit{}).Where("inbound_id = ? AND (reseller_id IS NULL OR reseller_id = 0)", old.InboundID).
				Update("reseller_id", account.ID).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: name}).Error
	})
}
