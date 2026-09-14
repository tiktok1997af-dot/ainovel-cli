package domain

import (
	"fmt"
	"unicode/utf8"
)

// ReviewInterval is the legacy/default global review interval. D07 production
// routing may supply a validated runtime checkpoint interval instead.
const ReviewInterval = 5

// ShouldReview preserves the historical default cadence for callers that do
// not carry D07 pipeline configuration.
func ShouldReview(completedCount int) (bool, string) {
	return ShouldReviewEvery(completedCount, ReviewInterval)
}

// ShouldReviewEvery evaluates the global-review checkpoint using an explicit
// positive interval. A non-positive interval falls back to ReviewInterval so
// old/recovery snapshots fail safe instead of dividing by zero.
func ShouldReviewEvery(completedCount, interval int) (bool, string) {
	if interval <= 0 {
		interval = ReviewInterval
	}
	if completedCount > 0 && completedCount%interval == 0 {
		return true, fmt.Sprintf("已完成 %d 章，触发全局审阅", completedCount)
	}
	return false, ""
}

// ShouldArcReview 长篇模式下判断是否需要弧级/卷级评审。
func ShouldArcReview(isArcEnd, isVolumeEnd bool, volume, arc int) (bool, string) {
	if isVolumeEnd {
		return true, fmt.Sprintf("第 %d 卷第 %d 弧结束（卷结束），触发弧级+卷级评审", volume, arc)
	}
	if isArcEnd {
		return true, fmt.Sprintf("第 %d 卷第 %d 弧结束，触发弧级评审", volume, arc)
	}
	return false, ""
}

// WordCount 按 rune 计算字数。
func WordCount(content string) int {
	return utf8.RuneCountInString(content)
}
