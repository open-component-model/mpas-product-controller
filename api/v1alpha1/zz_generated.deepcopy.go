//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Product) DeepCopyInto(out *Product) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Product.
func (in *Product) DeepCopy() *Product {
	if in == nil {
		return nil
	}
	out := new(Product)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Product) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProductDeploymentGenerator) DeepCopyInto(out *ProductDeploymentGenerator) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProductDeploymentGenerator.
func (in *ProductDeploymentGenerator) DeepCopy() *ProductDeploymentGenerator {
	if in == nil {
		return nil
	}
	out := new(ProductDeploymentGenerator)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProductDeploymentGenerator) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProductDeploymentGeneratorList) DeepCopyInto(out *ProductDeploymentGeneratorList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ProductDeploymentGenerator, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProductDeploymentGeneratorList.
func (in *ProductDeploymentGeneratorList) DeepCopy() *ProductDeploymentGeneratorList {
	if in == nil {
		return nil
	}
	out := new(ProductDeploymentGeneratorList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProductDeploymentGeneratorList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProductDeploymentGeneratorSpec) DeepCopyInto(out *ProductDeploymentGeneratorSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProductDeploymentGeneratorSpec.
func (in *ProductDeploymentGeneratorSpec) DeepCopy() *ProductDeploymentGeneratorSpec {
	if in == nil {
		return nil
	}
	out := new(ProductDeploymentGeneratorSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProductDeploymentGeneratorStatus) DeepCopyInto(out *ProductDeploymentGeneratorStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProductDeploymentGeneratorStatus.
func (in *ProductDeploymentGeneratorStatus) DeepCopy() *ProductDeploymentGeneratorStatus {
	if in == nil {
		return nil
	}
	out := new(ProductDeploymentGeneratorStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProductList) DeepCopyInto(out *ProductList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Product, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProductList.
func (in *ProductList) DeepCopy() *ProductList {
	if in == nil {
		return nil
	}
	out := new(ProductList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProductList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProductSpec) DeepCopyInto(out *ProductSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProductSpec.
func (in *ProductSpec) DeepCopy() *ProductSpec {
	if in == nil {
		return nil
	}
	out := new(ProductSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProductStatus) DeepCopyInto(out *ProductStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProductStatus.
func (in *ProductStatus) DeepCopy() *ProductStatus {
	if in == nil {
		return nil
	}
	out := new(ProductStatus)
	in.DeepCopyInto(out)
	return out
}
