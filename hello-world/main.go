package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ChimeraCoder/anaconda"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/guregu/dynamo"
)

// User つぶやいたユーザ情報
type User struct {
	ID         int64  `dynamo:"id"`
	Name       string `dynamo:"name"`
	ScreenName string `dynamo:"screen_name"`
}

// Tweet 参加を募集するツイート
type Tweet struct {
	ID        int64    `dynamo:"tweet_id"` //パーティションキー
	FullText  string   `dynamo:"full_text"`
	TweetedAt int64    `dynamo:"tweeted_at"` //dynamodbでソート出来るようにUNIX時間
	IsClub    bool     `dynamo:"is_club"`
	Positions []string `dynamo:"position"`
	MediaURLs []string `dynamo:"media_url"`
	User      User     `dynamo:"user"`
}

// Tweets 構造体のスライス
type Tweets []Tweet

// 以下インタフェースを渡してTweetedAtでソート可能にする
func (t Tweets) Len() int {
	return len(t)
}

func (t Tweets) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (t Tweets) Less(i, j int) bool {
	return t[i].TweetedAt < t[j].TweetedAt
}

// NewTweet Tweetのメンバを初期化する関数
func NewTweet() Tweet {
	tweet := Tweet{}
	tweet.ID = 0
	tweet.FullText = ""
	tweet.TweetedAt = 0
	tweet.IsClub = false
	tweet.Positions = []string{}
	tweet.User = User{}
	tweet.MediaURLs = []string{}
	return tweet
}

func uniq(strSlice []string) []string {
	m := make(map[string]bool)
	uniq := []string{}

	for _, ele := range strSlice {
		if !m[ele] {
			m[ele] = true
			uniq = append(uniq, ele)
		}
	}
	return uniq
}
func crawlTweets() {

	// dynamoDBに保存しておくツイート数
	const tweetsLimit = 100

	const tableName = "proclub_tweets"

	// 募集のツイートかどうかを判定する単語
	const isClubDecideWord = "募集"

	// 認証
	creds := credentials.NewStaticCredentials(os.Getenv("AWS_ACCEESS_KEY"), os.Getenv("AWS_SECRET_ACCEESS_KEY"), "") //第３引数はtoken

	sess, _ := session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String("ap-northeast-1")},
	)

	db := dynamo.New(sess)
	table := db.Table(tableName)

	anaconda.SetConsumerKey(os.Getenv("CONSUMER_KEY"))
	anaconda.SetConsumerSecret(os.Getenv("CONSUMER_SECRET"))

	api := anaconda.NewTwitterApi(os.Getenv("ACCESS_TOKEN"), os.Getenv("ACCESS_TOKEN_SECRET"))

	v := url.Values{}
	v.Set("tweet_mode", "extended")

	searchResult, _ := api.GetSearch("#プロクラブ", v)

	// 文字列→日付に変換するレイアウト
	var layout = "Mon Jan 2 15:04:05 +0000 2006"

	for _, tweet := range searchResult.Statuses {

		// 画像や動画などのメディアのURLを配列に入れる
		mediaURLs := []string{}

		newTweet := NewTweet()
		tweetedTime, _ := time.Parse(layout, tweet.CreatedAt)

		// リツイートされたものは日付だけアップデート
		if tweet.RetweetedStatus == nil {
			// メディアのURLを本文から削除
			fullTextRemovedURL := tweet.FullText

			for _, url := range tweet.Entities.Urls {
				mediaURLs = append(mediaURLs, url.Expanded_url)
				fullTextRemovedURL = strings.Replace(fullTextRemovedURL, url.Url, "", -1)
			}
			log.Println(tweet.Entities.Urls)
			tweetID, _ := strconv.ParseInt(tweet.IdStr, 10, 64)
			newTweet.ID = tweetID
			newTweet.FullText = fullTextRemovedURL
			newTweet.TweetedAt = tweetedTime.Unix()
			newTweet.IsClub = strings.Contains(fullTextRemovedURL, isClubDecideWord)
			newTweet.Positions = searchPositions(fullTextRemovedURL)
			newTweet.MediaURLs = mediaURLs
			newTweet.User = User{
				tweet.User.Id,
				tweet.User.Name,
				tweet.User.ScreenName,
			}

			if err := table.Put(newTweet).If("attribute_not_exists(tweet_id)").Run(); err != nil {
				log.Println(err.Error())
			} else {
				log.Println("成功！")
			}
		} else {
			fullTextRemovedURL := tweet.RetweetedStatus.FullText

			for _, url := range tweet.RetweetedStatus.Entities.Urls {
				mediaURLs = append(mediaURLs, url.Expanded_url)
				fullTextRemovedURL = strings.Replace(fullTextRemovedURL, url.Url, "", -1)
			}
			log.Println(tweet.Entities.Urls)

			tweetID, _ := strconv.ParseInt(tweet.RetweetedStatus.IdStr, 10, 64)

			newTweet.ID = tweetID
			newTweet.FullText = fullTextRemovedURL
			newTweet.TweetedAt = tweetedTime.Unix()
			newTweet.IsClub = strings.Contains(fullTextRemovedURL, isClubDecideWord)
			newTweet.Positions = searchPositions(fullTextRemovedURL)
			newTweet.MediaURLs = mediaURLs
			newTweet.User = User{
				tweet.RetweetedStatus.User.Id,
				tweet.RetweetedStatus.User.Name,
				tweet.RetweetedStatus.User.ScreenName,
			}

			if err := table.Put(newTweet).Run(); err != nil {
				log.Println(err.Error())
			} else {
				log.Println("成功！")
			}
		}
	}

	// 100件より多かったら最新100件に削除する
	var tweets Tweets

	err := table.Scan().All(&tweets)
	if err != nil {
		fmt.Println("err")
		panic(err.Error())
	}

	// IDの昇順でソート
	sort.Sort(tweets)

	var willDeleteCount = tweets.Len() - tweetsLimit

	fmt.Println("ツイート数: ", tweets.Len())

	for i := 0; i < willDeleteCount; i++ {
		err := table.Delete("tweet_id", tweets[i].ID).
			Run()
		if err != nil {
			fmt.Println("err")
			panic(err.Error())
		}
		fmt.Println("delete tweetID: ", tweets[i].ID)
	}

}

// 募集ツイートから募集しているポジションを探す
func searchPositions(text string) []string {
	r := regexp.MustCompile(`ST|RW|LW|CF|LM|CM|CDM|CAM|RM|LB|CB|RB|GK|SB|WG|DF|ディフェンダー`)
	foundPositions := []string{}
	results := r.FindAllStringSubmatch(text, -1)

	// [][]stringで返ってくるので[]stringに直す
	for _, result := range results {
		for _, word := range result {
			if word == "SB" {
				foundPositions = append(foundPositions, "RB")
				foundPositions = append(foundPositions, "LB")
			} else if word == "WG" {
				foundPositions = append(foundPositions, "RW")
				foundPositions = append(foundPositions, "LW")
			} else if word == "DF" || word == "ディフェンダー" {
				foundPositions = append(foundPositions, "RB")
				foundPositions = append(foundPositions, "LB")
				foundPositions = append(foundPositions, "CB")
			} else {
				foundPositions = append(foundPositions, word)
			}
		}
	}

	// 重複削除して返す
	return uniq(foundPositions)
}

func main() {
	// ラムダ実行
	lambda.Start(crawlTweets)
}
