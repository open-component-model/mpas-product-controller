package fakes

import (
	"context"
	"fmt"

	"github.com/open-component-model/ocm/pkg/contexts/ocm"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	ocmctl "github.com/open-component-model/mpas-product-controller/pkg/ocm"
)

// getResourceDataReturnValues defines the return values of the GetResourceData function.
type getResourceDataReturnValues struct {
	data []byte
	err  error
}

// getProductDescriptionReturnValues defines the return values of the GetProductDescription function.
type getProductDescriptionReturnValues struct {
	data []byte
	err  error
}

type MockOCM struct {
	getResourceDataCallCount  int
	getResourceDataReturns    map[int]getResourceDataReturnValues
	getResourceDataCalledWith [][]any

	getComponentVersionMap        map[string]ocm.ComponentVersionAccess
	getComponentVersionErr        error
	getComponentVersionCalledWith [][]any

	getProductDescriptionVersionCallCount int
	getProductDescriptionDataReturns      map[int]getProductDescriptionReturnValues
	getProductDescriptionDataCalledWith   [][]any
}

var _ ocmctl.Contract = &MockOCM{}

func (m *MockOCM) CreateAuthenticatedOCMContext(ctx context.Context, serviceAccountName, namespace string) (ocm.Context, error) {
	return ocm.New(), nil
}

func (m *MockOCM) GetComponentVersion(ctx context.Context, octx ocm.Context, url, name, version string) (ocm.ComponentVersionAccess, error) {
	m.getComponentVersionCalledWith = append(m.getComponentVersionCalledWith, []any{url, name, version})
	return m.getComponentVersionMap[name], m.getComponentVersionErr
}

func (m *MockOCM) GetComponentVersionReturnsForName(name string, cva ocm.ComponentVersionAccess, err error) {
	if m.getComponentVersionMap == nil {
		m.getComponentVersionMap = make(map[string]ocm.ComponentVersionAccess)
	}
	m.getComponentVersionMap[name] = cva
	m.getComponentVersionErr = err
}

func (m *MockOCM) GetComponentVersionCallingArgumentsOnCall(i int) []any {
	return m.getComponentVersionCalledWith[i]
}

func (m *MockOCM) GetComponentVersionWasNotCalled() bool {
	return len(m.getComponentVersionCalledWith) == 0
}

func (m *MockOCM) GetProductDescription(ctx context.Context, octx ocm.Context, cv ocm.ComponentVersionAccess) ([]byte, error) {
	if _, ok := m.getProductDescriptionDataReturns[m.getResourceDataCallCount]; !ok {
		return nil, fmt.Errorf("unexpected number of calls; not enough return values have been configured; call count %d", m.getProductDescriptionVersionCallCount)
	}
	m.getProductDescriptionDataCalledWith = append(m.getProductDescriptionDataCalledWith, []any{cv})
	result := m.getProductDescriptionDataReturns[m.getProductDescriptionVersionCallCount]
	m.getProductDescriptionVersionCallCount++

	return result.data, result.err
}

func (m *MockOCM) GetProductDescriptionReturns(data []byte, err error) {
	if m.getProductDescriptionDataReturns == nil {
		m.getProductDescriptionDataReturns = make(map[int]getProductDescriptionReturnValues)
	}

	m.getProductDescriptionDataReturns[0] = getProductDescriptionReturnValues{
		data: data,
		err:  err,
	}
}

func (m *MockOCM) GetResourceDataReturns(data []byte, err error) {
	if m.getResourceDataReturns == nil {
		m.getResourceDataReturns = make(map[int]getResourceDataReturnValues)
	}
	m.getResourceDataReturns[0] = getResourceDataReturnValues{
		data: data,
		err:  err,
	}
}

func (m *MockOCM) GetResourceDataReturnsOnCall(n int, data []byte, err error) {
	if m.getResourceDataReturns == nil {
		m.getResourceDataReturns = make(map[int]getResourceDataReturnValues, 0)
	}

	m.getResourceDataReturns[n] = getResourceDataReturnValues{
		data: data,
		err:  err,
	}
}

func (m *MockOCM) GetResourceData(cv ocm.ComponentVersionAccess, ref v1alpha1.ResourceReference) ([]byte, error) {
	if _, ok := m.getResourceDataReturns[m.getResourceDataCallCount]; !ok {
		return nil, fmt.Errorf("unexpected number of calls; not enough return values have been configured; call count %d", m.getResourceDataCallCount)
	}
	m.getResourceDataCalledWith = append(m.getResourceDataCalledWith, []any{ref})
	result := m.getResourceDataReturns[m.getResourceDataCallCount]
	m.getResourceDataCallCount++
	return result.data, result.err
}
