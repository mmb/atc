port module LoginPage exposing (..)

import Navigation
import String
import UrlParser exposing ((</>), s)

import Login exposing (Page (..))

main : Program Login.Flags
main =
  Navigation.programWithFlags
    (Navigation.makeParser pathnameParser)
    { init = Login.init
    , update = Login.update
    , urlUpdate = Login.urlUpdate
    , view = Login.view
    , subscriptions = always Sub.none
    }

pathnameParser : Navigation.Location -> Result String Page
pathnameParser location =
  UrlParser.parse identity pageParser (String.dropLeft 1 location.pathname)

pageParser : UrlParser.Parser (Page -> a) a
pageParser =
  UrlParser.oneOf
    [ UrlParser.format TeamSelectionPage (s "login")
    , UrlParser.format LoginPage (s "teams" </> UrlParser.string </> s "login")
    ]
