package keeper_test

import (
	"strconv"

	"github.com/Stride-Labs/stride/v4/x/ratelimit/keeper"
	"github.com/Stride-Labs/stride/v4/x/ratelimit/types"
)

func (s *KeeperTestSuite) createPaths() []types.Path {
	paths := []types.Path{}
	for i := 1; i <= 5; i++ {
		path := types.Path{
			Id: strconv.Itoa(i),
		}

		paths = append(paths, path)
		s.App.RatelimitKeeper.SetPath(s.Ctx, path)
	}
	return paths
}

func (s *KeeperTestSuite) TestFormatPath() {
	s.Require().Equal(keeper.FormatPathId("denom", "channel-0"), "denom/channel-0")
}

func (s *KeeperTestSuite) TestGetPath() {
	paths := s.createPaths()
	expectedPath := paths[0]

	actualPath, found := s.App.RatelimitKeeper.GetPath(s.Ctx, expectedPath.Id)
	s.Require().True(found, "element should have been found, but was not")
	s.Require().Equal(expectedPath, actualPath)
}

func (s *KeeperTestSuite) TestRemovePath() {
	paths := s.createPaths()
	idToRemove := paths[0].Id

	s.App.RatelimitKeeper.RemovePath(s.Ctx, idToRemove)
	_, found := s.App.RatelimitKeeper.GetPath(s.Ctx, idToRemove)
	s.Require().False(found, "element should not have been found, but was")
}

func (s *KeeperTestSuite) TestGetAllPaths() {
	expectedPaths := s.createPaths()
	actualPaths := s.App.RatelimitKeeper.GetAllPaths(s.Ctx)
	s.Require().Len(actualPaths, len(expectedPaths))
	s.Require().ElementsMatch(expectedPaths, actualPaths, "all paths")
}