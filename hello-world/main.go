package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"sort"
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
	ID   int64  `dynamo:"id"`
	Name string `dynamo:"name"`
}

// Tweet 参加を募集するツイート
type Tweet struct {
	ID        int64    `dynamo:"tweet_id"` //パーティションキー
	FullText  string   `dynamo:"full_text"`
	TweetedAt int64    `dynamo:"tweeted_at"` //dynamodbでソート出来るようにUNIX時間
	IsClub    bool     `dynamo:"is_club"`
	Positions []string `dynamo:"postion"`
	User      User     `dynamo:"user"`
}

// Tweets 構造体のスライス
type Tweets []Tweet

// 以下インタフェースを渡してTweetIDでソート可能にするｓ
func (t Tweets) Len() int {
	return len(t)
}

func (t Tweets) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (t Tweets) Less(i, j int) bool {
	return t[i].ID < t[j].ID
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
		tweetedTime, _ := time.Parse(layout, tweet.CreatedAt)
		// リツイートされたものは除く
		if tweet.RetweetedStatus == nil {
			newTweet := Tweet{
				tweet.Id,
				tweet.FullText,
				tweetedTime.Unix(),
				strings.Contains(tweet.FullText, isClubDecideWord),
				searchPositions(tweet.FullText),
				User{
					tweet.User.Id,
					tweet.User.Name,
				},
			}

			if err := table.Put(newTweet).Run(); err != nil {
				log.Println(err.Error())
			} else {
				log.Println("成功！")
			}
		} else {
			newTweet := Tweet{
				tweet.RetweetedStatus.Id,
				tweet.RetweetedStatus.FullText,
				tweetedTime.Unix(),
				strings.Contains(tweet.FullText, isClubDecideWord),
				searchPositions(tweet.FullText),
				User{
					tweet.User.Id,
					tweet.User.Name,
				},
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
			Range("tweeted_at", tweets[i].TweetedAt).
			Run()
		if err != nil {
			fmt.Println("err")
			panic(err.Error())
		}
		fmt.Println("delete tweetID: ", tweets[i].ID)
	}

	searchPositions("本日22時半から1時間ほど体験募集してます。\n\n募集ポジション　CM CDM CB\n\nDMお待ちしております。\n#FIFA21  #プロクラブ")
}

func searchPositions(text string) []string {
	r := regexp.MustCompile(`ST|RW|LW|CF|LM|CM|CDM|CAM|RM|LB|CB|RB|GK`)
	foundPositions := []string{}
	results := r.FindAllStringSubmatch(text, -1)

	// [][]stringで返ってくるので[]stringに直す
	for _, result := range results {
		for _, word := range result {
			foundPositions = append(foundPositions, word)
		}
	}
	log.Println(foundPositions)
	return foundPositions
}

func main() {
	// ラムダ実行
	lambda.Start(crawlTweets)
}
